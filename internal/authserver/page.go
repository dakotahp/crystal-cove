package authserver

import (
	"html/template"
	"net/http"
)

type pageData struct {
	ClientName string
	ReturnHost string
	Fields     map[string]string
	Message    string
	Error      string
}

var signInPage = template.Must(template.New("signin").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sign in to Crystal Cove</title>
<style>
body{font-family:system-ui,sans-serif;max-width:26rem;margin:3rem auto;padding:0 1rem;color:#1f1d2b;background:#fff}
@media (prefers-color-scheme:dark){body{color:#ece9f5;background:#16141f}}
input,button{font:inherit;width:100%;box-sizing:border-box;padding:.6rem;margin:.4rem 0}
.message{color:#c4314b}
</style>
</head>
<body>
<h1>Crystal Cove</h1>
{{if .Error}}<p class="message">{{.Error}}</p>{{else}}
<p><strong>{{.ClientName}}</strong> wants access to your vault.</p>
<p>After you sign in, you return to <strong>{{.ReturnHost}}</strong>.</p>
{{with .Message}}<p class="message">{{.}}</p>{{end}}
<form method="post" action="/authorize">
{{range $name, $value := .Fields}}<input type="hidden" name="{{$name}}" value="{{$value}}">
{{end}}<label for="password">Owner password</label>
<input id="password" name="password" type="password" autocomplete="current-password" required autofocus>
<button type="submit">Allow access</button>
</form>{{end}}
</body>
</html>
`))

// returnOrigin is allowed in form-action because browsers apply it to the
// redirect that follows the form post, which leaves for the client.
func renderPage(w http.ResponseWriter, status int, returnOrigin string, data pageData) {
	formAction := "'self'"
	if returnOrigin != "" {
		formAction += " " + returnOrigin
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action "+formAction+"; frame-ancestors 'none'")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = signInPage.Execute(w, data)
}

func renderError(w http.ResponseWriter, status int, message string) {
	renderPage(w, status, "", pageData{Error: message})
}
