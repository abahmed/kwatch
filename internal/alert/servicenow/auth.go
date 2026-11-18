package servicenow

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// tokenPath is ServiceNow's OAuth token endpoint.
const tokenPath = "/oauth_token.do"

// tokenMargin renews a token this long before it expires, so a request
// never starts with a token about to lapse.
const tokenMargin = time.Minute

// defaultTokenLife is assumed when the token answer names no lifetime.
const defaultTokenLife = 30 * time.Minute

// tokens fetches and caches an OAuth client-credentials access token.
type tokens struct {
	mu      sync.Mutex
	token   string
	expires time.Time
}

// authorize adds the configured credentials to r.
func (p *ServiceNow) authorize(
	ctx context.Context, r *transport.Request,
) error {
	if !p.settings.oauth() {
		r.BasicAuth = &transport.BasicAuth{
			Username: p.settings.user, Password: p.settings.password,
		}
		return nil
	}
	token, err := p.accessToken(ctx)
	if err != nil {
		return &tokenError{err: err}
	}
	r.Headers["Authorization"] = "Bearer " + token
	return nil
}

// accessToken returns a cached token or fetches a new one. The lock is
// held across the fetch so concurrent sends share one token request.
func (p *ServiceNow) accessToken(ctx context.Context) (string, error) {
	p.tokens.mu.Lock()
	defer p.tokens.mu.Unlock()
	now := p.now()
	if p.tokens.token != "" && now.Before(p.tokens.expires) {
		return p.tokens.token, nil
	}
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {p.settings.clientID},
		"client_secret": {p.settings.clientSecret},
	}
	answer, err := p.sender.Send(ctx, transport.Request{
		Provider: p.Name(), Method: "POST",
		URL:         p.settings.base + tokenPath,
		Body:        []byte(form.Encode()),
		ContentType: "application/x-www-form-urlencoded",
		Headers:     map[string]string{"Accept": "application/json"},
	})
	if err != nil {
		return "", err
	}
	var granted struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if json.Unmarshal(answer, &granted) != nil ||
		granted.AccessToken == "" {
		return "", transport.Permanent(errors.New(
			"servicenow token endpoint returned no access token"))
	}
	life := time.Duration(granted.ExpiresIn) * time.Second
	if life <= 0 {
		life = defaultTokenLife
	}
	p.tokens.token = granted.AccessToken
	p.tokens.expires = now.Add(max(life-tokenMargin, life/2))
	return p.tokens.token, nil
}

// tokenError is a failed token request. It keeps the cause's status and
// permanence for the delivery's classification.
type tokenError struct{ err error }

func (e *tokenError) Error() string {
	return "servicenow OAuth token request: " + e.err.Error()
}

func (e *tokenError) Unwrap() error { return e.err }

// dropToken forgets the cached token after the instance refused it, so
// the next request fetches a new one.
func (p *ServiceNow) dropToken() {
	p.tokens.mu.Lock()
	defer p.tokens.mu.Unlock()
	p.tokens.token = ""
}
