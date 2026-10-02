package authserver

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"
)

type authRequest struct {
	client      *client
	redirectURI string
	state       string
	challenge   string
}

type authError struct {
	code        string
	description string
	redirect    bool
}

var authorizeFields = []string{"response_type", "client_id", "redirect_uri", "state",
	"code_challenge", "code_challenge_method", "scope", "resource"}

// Errors with redirect false come before the redirect URI is trusted and must
// never redirect. Errors with redirect true come with a non-nil request.
func (s *Server) parseAuthRequest(form url.Values) (*authRequest, *authError) {
	c, ok := s.store.client(form.Get("client_id"))
	if !ok {
		return nil, &authError{"invalid_client", "This app is not registered with this server. Connect it again.", false}
	}
	redirectURI := form.Get("redirect_uri")
	if redirectURI == "" && len(c.RedirectURIs) == 1 {
		redirectURI = c.RedirectURIs[0]
	}
	if !slices.Contains(c.RedirectURIs, redirectURI) {
		return nil, &authError{"invalid_request", "The return address does not match the app's registration.", false}
	}
	req := &authRequest{client: c, redirectURI: redirectURI, state: form.Get("state"), challenge: form.Get("code_challenge")}
	switch {
	case form.Get("response_type") != "code":
		return req, &authError{"unsupported_response_type", "response_type must be code", true}
	case req.challenge == "" || form.Get("code_challenge_method") != "S256":
		return req, &authError{"invalid_request", "PKCE with code_challenge_method=S256 is required", true}
	case form.Get("resource") != "" && !s.matchesResource(form.Get("resource")):
		return req, &authError{"invalid_target", "resource must be " + s.publicURL, true}
	}
	return req, nil
}

func (s *Server) failAuthorize(w http.ResponseWriter, r *http.Request, req *authRequest, aerr *authError) {
	if !aerr.redirect {
		renderError(w, http.StatusBadRequest, aerr.description)
		return
	}
	s.redirect(w, r, req, url.Values{"error": {aerr.code}, "error_description": {aerr.description}})
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request, req *authRequest, params url.Values) {
	u, _ := url.Parse(req.redirectURI) // validated at registration
	q := u.Query()
	for k, vs := range params {
		q[k] = vs
	}
	if req.state != "" {
		q.Set("state", req.state)
	}
	q.Set("iss", s.publicURL)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (s *Server) pageFor(req *authRequest, form url.Values) (string, pageData) {
	u, _ := url.Parse(req.redirectURI) // validated at registration
	data := pageData{ClientName: req.client.Name, ReturnHost: u.Host, Fields: make(map[string]string)}
	if data.ClientName == "" {
		data.ClientName = "Unnamed client"
	}
	for _, name := range authorizeFields {
		if v := form.Get(name); v != "" {
			data.Fields[name] = v
		}
	}
	data.Fields["redirect_uri"] = req.redirectURI
	return u.Scheme + "://" + u.Host, data
}

func (s *Server) authorizePage(w http.ResponseWriter, r *http.Request) {
	form := r.URL.Query()
	req, aerr := s.parseAuthRequest(form)
	if aerr != nil {
		s.failAuthorize(w, r, req, aerr)
		return
	}
	origin, data := s.pageFor(req, form)
	renderPage(w, http.StatusOK, origin, data)
}

func (s *Server) authorizeSubmit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		renderError(w, http.StatusBadRequest, "The sign-in form could not be read.")
		return
	}
	form := r.PostForm
	req, aerr := s.parseAuthRequest(form)
	if aerr != nil {
		s.failAuthorize(w, r, req, aerr)
		return
	}
	origin, data := s.pageFor(req, form)
	sum := sha256.Sum256([]byte(form.Get("password")))
	wait, ok := s.limiter.attempt(func() bool {
		return subtle.ConstantTimeCompare(sum[:], s.passwordHash[:]) == 1
	})
	if wait > 0 {
		data.Message = fmt.Sprintf("Too many wrong passwords. Try again in %s.", wait.Round(time.Second))
		renderPage(w, http.StatusTooManyRequests, origin, data)
		return
	}
	if !ok {
		s.audit.Warn("sign-in failed: wrong password", "client_id", req.client.ID, "remote_addr", r.RemoteAddr)
		data.Message = "Wrong password."
		renderPage(w, http.StatusUnauthorized, origin, data)
		return
	}

	code, err := newToken(s.rand)
	if err != nil {
		s.audit.Error("sign-in server error", "error", err.Error())
		renderError(w, http.StatusInternalServerError, "The server could not finish the sign-in. See the server log.")
		return
	}
	s.mu.Lock()
	s.pruneLocked()
	s.codes[hashToken(code)] = &pendingCode{
		clientID:    req.client.ID,
		redirectURI: req.redirectURI,
		challenge:   req.challenge,
		expires:     s.now().Add(codeLifetime),
	}
	s.mu.Unlock()
	s.audit.Info("sign-in succeeded", "client_id", req.client.ID, "client_name", req.client.Name)
	s.redirect(w, r, req, url.Values{"code": {code}})
}
