package dashboard

import "strings"

// authSecrets are the sign-in secrets as they are compared.
type authSecrets struct {
	token string
	// password is normalized (normalizePassword); rawPassword is as
	// configured.
	password    string
	rawPassword string
	// grouped marks a password in the generated grouped form, which is
	// accepted in any case and with or without its dashes.
	grouped bool
}

func newAuthSecrets(token, password string) *authSecrets {
	return &authSecrets{
		token:       token,
		password:    normalizePassword(password),
		rawPassword: password,
		grouped: groupedPassword.MatchString(
			strings.TrimSpace(password)),
	}
}

// configured reports whether there is a token or a password. Without
// either every request is refused.
func (s *authSecrets) configured() bool {
	return s.token != "" || s.password != ""
}

func (a *Auth) secret() *authSecrets { return a.secrets.Load() }

// setCredentials replaces the sign-in secrets. Sessions of the old ones
// are signed out.
func (a *Auth) setCredentials(token, password string) {
	a.secrets.Store(newAuthSecrets(token, password))
	a.sessions.rekey(normalizePassword(password), token)
}

// SetCredentials replaces the bearer token and the sign-in password of a
// running server. The application calls it when the credentials could
// not be read at start and a later try succeeded, so one failed read
// does not lock the dashboard until the next restart.
func (s *Server) SetCredentials(token, password string) {
	s.auth.setCredentials(token, password)
}

// HasCredentials reports whether a token or a password is set. Without
// either every request is refused.
func (s *Server) HasCredentials() bool {
	return s.auth.secret().configured()
}
