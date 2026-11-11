package awscreds

import (
	"context"
	"errors"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/signing"
)

// Source hands out the credentials a request is signed with.
// Invalidate drops cached credentials, so the next call fetches new
// ones; it does nothing for static keys.
type Source interface {
	Credentials(ctx context.Context) (signing.Credentials, error)
	Invalidate()
}

// Static is a Source of fixed keys from the configuration.
type Static struct {
	Keys signing.Credentials
}

// Credentials returns the configured keys.
func (s Static) Credentials(context.Context) (signing.Credentials, error) {
	return s.Keys, nil
}

// Invalidate does nothing: static keys cannot be fetched again.
func (Static) Invalidate() {}

// refreshBefore is how long before their expiry cached credentials are
// fetched again, so a request never goes out with keys about to lapse.
const refreshBefore = 5 * time.Minute

// fetchFunc fetches new temporary credentials and their expiry.
type fetchFunc func(context.Context) (signing.Credentials, time.Time,
	error)

// cached is a Source that keeps fetched credentials until shortly
// before they expire.
type cached struct {
	name  string
	fetch fetchFunc
	now   func() time.Time

	mu      sync.Mutex
	creds   signing.Credentials
	expires time.Time
	valid   bool
}

func newCached(name string, now func() time.Time, fetch fetchFunc) *cached {
	return &cached{name: name, fetch: fetch,
		now: clock.RequireFunc(now)}
}

// errNoExpiration is returned for an answer without an expiry: kwatch
// could not tell when to fetch again.
var errNoExpiration = errors.New("credentials have no expiration")

// Credentials returns the cached credentials, fetching new ones when
// none are cached or they expire within refreshBefore. A failed fetch
// keeps serving credentials that have not expired yet.
func (c *cached) Credentials(
	ctx context.Context,
) (signing.Credentials, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if c.valid && now.Before(c.expires.Add(-refreshBefore)) {
		return c.creds, nil
	}
	creds, expires, err := c.fetch(ctx)
	if err == nil && expires.IsZero() {
		err = errNoExpiration
	}
	if err != nil {
		if c.valid && now.Before(c.expires) {
			return c.creds, nil
		}
		return signing.Credentials{}, err
	}
	c.creds, c.expires, c.valid = creds, expires, true
	klog.InfoS("fetched AWS credentials", "source", c.name,
		"expires", expires.UTC().Format(time.RFC3339))
	return creds, nil
}

// Invalidate drops the cached credentials.
func (c *cached) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.valid = false
	c.creds = signing.Credentials{}
}

// Refreshable reports whether src can fetch new credentials, which
// static keys cannot.
func Refreshable(src Source) bool {
	switch src.(type) {
	case Static, *Static, nil:
		return false
	}
	return true
}
