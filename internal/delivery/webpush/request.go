package webpush

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// Subscription is one browser, as its PushSubscription.toJSON() gives
// it: where its push service takes messages and its keys.
type Subscription struct {
	Endpoint string `json:"endpoint"`
	Keys     Keys   `json:"keys"`
}

// maxEndpoint bounds a push service address.
const maxEndpoint = 1024

// pushHosts and pushDomains are the push services of the browsers
// kwatch supports: Chrome and the browsers built on it, Firefox,
// Safari and Edge.
var (
	pushHosts = []string{"fcm.googleapis.com",
		"updates.push.services.mozilla.com"}
	pushDomains = []string{".push.apple.com", ".notify.windows.com"}
)

// ErrEndpoint says an address is not a known push service.
var ErrEndpoint = errors.New("webpush: not a known push service address")

// ParseEndpoint checks a push service address: https on the default
// port, no user info, at most 1 KiB, at a known push service.
func ParseEndpoint(raw string) (*url.URL, error) {
	if len(raw) > maxEndpoint {
		return nil, ErrEndpoint
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil ||
		u.Port() != "" || u.Opaque != "" {
		return nil, ErrEndpoint
	}
	host := strings.ToLower(u.Hostname())
	for _, h := range pushHosts {
		if host == h {
			return u, nil
		}
	}
	for _, d := range pushDomains {
		if strings.HasSuffix(host, d) && len(host) > len(d) {
			return u, nil
		}
	}
	return nil, ErrEndpoint
}

// Valid reports whether sub can be sent to.
func (s Subscription) Valid() bool {
	_, err := ParseEndpoint(s.Endpoint)
	return err == nil && s.Keys.Valid()
}

// Request builds the message carrying payload to sub, signed with key.
// ttl is how long the push service keeps it for a browser that is
// offline; urgency high wakes a device that is saving power.
func Request(
	sub Subscription, payload []byte, key Key, now time.Time,
	ttl time.Duration,
) (transport.Request, error) {
	u, err := ParseEndpoint(sub.Endpoint)
	if err != nil {
		return transport.Request{}, err
	}
	body, err := Encrypt(sub.Keys, payload)
	if err != nil {
		return transport.Request{}, err
	}
	auth, err := key.authorization(u, now)
	if err != nil {
		return transport.Request{}, err
	}
	return transport.Request{
		Provider:    "webpush",
		Method:      http.MethodPost,
		URL:         u.String(),
		Body:        body,
		ContentType: "application/octet-stream",
		Headers: map[string]string{
			"Authorization":    auth,
			"Content-Encoding": "aes128gcm",
			"TTL":              strconv.Itoa(int(ttl / time.Second)),
			"Urgency":          "high",
		},
	}, nil
}

// Gone reports whether err says the browser's subscription no longer
// exists (404 or 410), so it should be forgotten.
func Gone(err error) bool {
	code, ok := transport.StatusOf(err)
	return ok && (code == http.StatusNotFound || code == http.StatusGone)
}
