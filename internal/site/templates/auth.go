// Package templates holds html/template strings for site pages. P1b uses bare
// HTML; P1c migrates to templ + Tailwind + Datastar.
package templates

import "html/template"

// LoginPage renders the email-entry form.
var LoginPage = template.Must(template.New("login").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Log in — Acai</title>
</head>
<body>
  <h1>Log in</h1>
  {{if .Flash}}<p style="color: #b00">{{.Flash}}</p>{{end}}
  <form method="post" action="/users/log-in">
    <input type="hidden" name="{{.CSRFFieldName}}" value="{{.CSRFToken}}">
    <label>Email <input type="email" name="email" required autofocus></label>
    <button type="submit">Send magic link</button>
  </form>
</body>
</html>`))

// LoginPageData is the input for LoginPage.
type LoginPageData struct {
	Flash         string
	CSRFFieldName string
	CSRFToken     string
}

// LoginRequestedPage renders the "check your email" confirmation.
var LoginRequestedPage = template.Must(template.New("login_requested").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Check your email — Acai</title>
</head>
<body>
  <h1>Check your email</h1>
  <p>If <strong>{{.Email}}</strong> matches an account, you'll receive a magic-link shortly.</p>
  <p><a href="/users/log-in">Back</a></p>
</body>
</html>`))

// LoginRequestedPageData is the input for LoginRequestedPage.
type LoginRequestedPageData struct {
	Email string
}

// RegisterPage renders the new-account registration form.
var RegisterPage = template.Must(template.New("register").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Create account — Acai</title>
</head>
<body>
  <h1>Create account</h1>
  {{if .Flash}}<p style="color: #b00">{{.Flash}}</p>{{end}}
  <form method="post" action="/users/register">
    <input type="hidden" name="{{.CSRFFieldName}}" value="{{.CSRFToken}}">
    <label>Email <input type="email" name="email" required autofocus></label>
    <button type="submit">Create account</button>
  </form>
  <p>Already have an account? <a href="/users/log-in">Log in</a></p>
</body>
</html>`))

// RegisterPageData is the input for RegisterPage.
type RegisterPageData struct {
	Flash         string
	CSRFFieldName string
	CSRFToken     string
}

// RegisterRequestedPage renders the "check your email" confirmation after
// a registration attempt.
var RegisterRequestedPage = template.Must(template.New("register_requested").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Check your email — Acai</title>
</head>
<body>
  <h1>Check your email</h1>
  <p>If <strong>{{.Email}}</strong> is not already registered, you'll receive a magic-link shortly to confirm your account.</p>
  <p><a href="/users/log-in">Back to log in</a></p>
</body>
</html>`))

// RegisterRequestedPageData is the input for RegisterRequestedPage.
type RegisterRequestedPageData struct {
	Email string
}
