package authserver

import (
	"html/template"
	"net/http"
	"strconv"
	"time"
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
<meta name="color-scheme" content="light dark">
<title>Sign in to Crystal Cove</title>
<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<style>
:root{--bg:#eeecf7;--glow:rgba(139,92,246,.22);--ink:#1d1838;--muted:#5d5878;--line:#cfc9e6;--field:#fff;--accent:#5b21b6;--accent-hover:#4c1d95;--accent-ink:#fff;--alert:#9f1d3a;--alert-bg:#fae8ed;--ring:#8b5cf6}
@media (prefers-color-scheme:dark){:root{--bg:#0d0a22;--glow:rgba(167,139,250,.2);--ink:#ece9f8;--muted:#a7a2c6;--line:#342d5e;--field:#161233;--accent:#a78bfa;--accent-hover:#c4b5fd;--accent-ink:#160d36;--alert:#ffb0c0;--alert-bg:#2d1233;--ring:#c4b5fd}}
*{box-sizing:border-box}
html{background:var(--bg)}
body{margin:0;min-height:100vh;font:1rem/1.5 system-ui,-apple-system,"Segoe UI",sans-serif;color:var(--ink);background:radial-gradient(38rem 24rem at 50% -6rem,var(--glow),transparent 70%) no-repeat,var(--bg);-webkit-text-size-adjust:100%}
main{max-width:23rem;margin:0 auto;padding:clamp(2.5rem,9vh,5rem) 1.25rem 3rem}
header{text-align:center;margin-bottom:2.25rem}
.badge{display:block;width:5.5rem;height:5.5rem;margin:0 auto .875rem;filter:drop-shadow(0 .5rem 1.5rem rgba(91,33,182,.35))}
.wordmark{margin:0;font:600 1.0625rem/1.2 "Iowan Old Style","Palatino Linotype",Palatino,Georgia,serif;letter-spacing:.02em;color:var(--muted)}
h1{margin:0 0 .5rem;font:600 1.5rem/1.25 "Iowan Old Style","Palatino Linotype",Palatino,Georgia,serif;text-wrap:balance}
h1 strong{font-weight:700}
.note{margin:0 0 1.75rem;color:var(--muted)}
.note strong{color:var(--ink);font-weight:600}
.alert{margin:0 0 1.25rem;padding:.75rem 1rem;border-radius:.5rem;border-left:3px solid var(--alert);background:var(--alert-bg);color:var(--alert)}
label{display:block;margin-bottom:.375rem;font-weight:600;font-size:.9375rem}
input,button{display:block;width:100%;font:inherit;border-radius:.625rem}
input{padding:.75rem .875rem;color:var(--ink);background:var(--field);border:1px solid var(--line)}
button{margin-top:1rem;padding:.8125rem 1rem;border:0;font-weight:600;color:var(--accent-ink);background:var(--accent);cursor:pointer}
button:hover{background:var(--accent-hover)}
input:focus-visible,button:focus-visible{outline:2px solid var(--ring);outline-offset:2px}
@media (min-width:40rem) and (min-height:42rem){.badge{width:8.5rem;height:8.5rem;margin-bottom:1.125rem}.wordmark{font-size:1.25rem}header{margin-bottom:2.75rem}}
</style>
</head>
<body>
<main>
<header>
<svg class="badge" viewBox="0 0 200 200" aria-hidden="true" focusable="false">
<defs>
<radialGradient id="skyGrad" cx="50%" cy="30%" r="75%"><stop offset="0" stop-color="#2a1f5c"/><stop offset="1" stop-color="#0b0820"/></radialGradient>
<linearGradient id="seaGrad" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#1c2d5e"/><stop offset="1" stop-color="#0a1130"/></linearGradient>
<radialGradient id="glowGrad"><stop offset="0" stop-color="#a78bfa" stop-opacity=".55"/><stop offset="1" stop-color="#a78bfa" stop-opacity="0"/></radialGradient>
<radialGradient id="moonHalo"><stop offset=".45" stop-color="#f1f0d0" stop-opacity=".4"/><stop offset="1" stop-color="#f1f0d0" stop-opacity="0"/></radialGradient>
<linearGradient id="beamFade" x1="1" y1="0" x2="0" y2="0"><stop offset="0" stop-color="#f6e7a1" stop-opacity=".3"/><stop offset="1" stop-color="#f6e7a1" stop-opacity="0"/></linearGradient>
<clipPath id="badge"><circle cx="100" cy="100" r="92"/></clipPath>
</defs>
<g id="scene" clip-path="url(#badge)">
<rect id="sky" width="200" height="200" fill="url(#skyGrad)"/>
<g id="stars" fill="#fff" opacity=".8"><circle cx="40" cy="75" r="1.2"/><circle cx="72" cy="26" r="1"/><circle cx="120" cy="30" r="1.3"/><circle cx="150" cy="84" r=".9"/><circle cx="92" cy="20" r=".8"/></g>
<path id="moon" d="M143.92 38.16 A14 14 0 1 0 159.36 56.17 A12 12 0 0 1 143.92 38.16Z" fill="#f6ecc9"/>
<circle id="glow" cx="100" cy="98" r="50" fill="url(#glowGrad)"/>
<rect id="sea" y="138" width="200" height="62" fill="url(#seaGrad)"/>
<path id="cliff-left" d="M0 60 L22 58 L34 76 L50 84 L62 110 L72 124 L78 140 L0 140Z" fill="#05030d"/>
<path id="cliff-right" d="M200 70 L178 66 L164 82 L150 90 L140 112 L128 126 L122 140 L200 140Z" fill="#05030d"/>

<g id="crystal"><polygon points="100,52 78,92 100,100" fill="#ddd6fe"/><polygon points="100,52 122,92 100,100" fill="#a78bfa"/><polygon points="78,92 90,138 100,100" fill="#8b5cf6"/><polygon points="122,92 110,138 100,100" fill="#5b21b6"/><polygon points="90,138 110,138 100,100" fill="#6d28d9"/></g>
<g id="reflection" transform="translate(0,276) scale(1,-1)" opacity=".22"><polygon points="100,52 78,92 100,100" fill="#ddd6fe"/><polygon points="100,52 122,92 100,100" fill="#a78bfa"/><polygon points="78,92 90,138 100,100" fill="#8b5cf6"/><polygon points="122,92 110,138 100,100" fill="#5b21b6"/></g>
<g id="waves" stroke="#7c8ae0" fill="none" opacity=".5" stroke-width="1.5" stroke-linecap="round">
<path d="M30 152 q8 -4 16 0 t16 0"/><path d="M130 160 q8 -4 16 0 t16 0"/><path d="M60 176 q8 -4 16 0 t16 0"/>
</g>
</g>
<circle id="ring" cx="100" cy="100" r="92" fill="none" stroke="#c4b5fd" stroke-width="4"/>
</svg>
<p class="wordmark">Crystal Cove</p>
</header>
{{if .Error}}<h1>Sign-in stopped</h1>
<p class="alert" role="alert">{{.Error}}</p>
<p class="note">Go back to the app you are connecting and start again.</p>
{{else}}<h1><strong>{{.ClientName}}</strong> wants access to your vault</h1>
<p class="note">After you sign in, you return to <strong>{{.ReturnHost}}</strong>.</p>
{{with .Message}}<p class="alert" role="alert">{{.}}</p>
{{end}}<form method="post" action="/authorize">
{{range $name, $value := .Fields}}<input type="hidden" name="{{$name}}" value="{{$value}}">
{{end}}<label for="password">Owner password</label>
<input id="password" name="password" type="password" autocomplete="current-password" required autofocus>
<button type="submit">Allow access</button>
</form>
{{end}}</main>
</body>
</html>
`))

// formatWait rounds up so the page never tells someone to retry before the
// lock ends.
func formatWait(d time.Duration) string {
	secs := int((d + time.Second - 1) / time.Second)
	if secs < 60 {
		return plural(secs, "second")
	}
	return plural((secs+59)/60, "minute")
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}

// returnOrigin is allowed in form-action because browsers apply it to the
// redirect that follows the form post, which leaves for the client.
func renderPage(w http.ResponseWriter, status int, returnOrigin string, data pageData) {
	formAction := "'self'"
	if returnOrigin != "" {
		formAction += " " + returnOrigin
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; form-action "+formAction+"; frame-ancestors 'none'")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = signInPage.Execute(w, data)
}

func renderError(w http.ResponseWriter, status int, message string) {
	renderPage(w, status, "", pageData{Error: message})
}
