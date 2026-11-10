package email

import (
	"errors"
	"fmt"
	"net/smtp"
	"strings"
)

// auth picks the login mechanism. PLAIN is the default. A server that
// advertises AUTH without PLAIN but with LOGIN (Exchange and some
// relays) answers 504 to PLAIN, which is a permanent failure and would
// reject every mail, so LOGIN is used there.
func (c smtpConfig) auth(client *smtp.Client) smtp.Auth {
	if ok, mechanisms := client.Extension("AUTH"); ok &&
		!offers(mechanisms, "PLAIN") && offers(mechanisms, "LOGIN") {
		return &loginAuth{
			username: c.username, password: c.password, host: c.host,
		}
	}
	return smtp.PlainAuth("", c.username, c.password, c.host)
}

// offers reports whether the AUTH extension line names mechanism.
func offers(mechanisms, mechanism string) bool {
	for _, offered := range strings.Fields(mechanisms) {
		if strings.EqualFold(offered, mechanism) {
			return true
		}
	}
	return false
}

// loginAuth is the AUTH LOGIN mechanism: the server asks for the user
// name and then the password. Like PLAIN it sends the password as it
// is, so it has the same guard: only over TLS, or to localhost, and
// only to the configured host.
type loginAuth struct {
	username, password, host string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS && !isLocalhost(server.Name) {
		return "", nil, errors.New("unencrypted connection")
	}
	if server.Name != a.host {
		return "", nil, errors.New("wrong host name")
	}
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(prompt []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	asked := strings.ToLower(strings.TrimSpace(string(prompt)))
	switch {
	case strings.HasPrefix(asked, "username"):
		return []byte(a.username), nil
	case strings.HasPrefix(asked, "password"):
		return []byte(a.password), nil
	}
	return nil, fmt.Errorf("unexpected AUTH LOGIN prompt %q", asked)
}

func isLocalhost(name string) bool {
	return name == "localhost" || name == "127.0.0.1" || name == "::1"
}
