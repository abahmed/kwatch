package dashboard

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A stream whose session ends says so at the next check, so the page
// can ask for sign-in at once, then closes.
func TestLiveSendsSignedOutWhenTheSessionEnds(t *testing.T) {
	g := newLiveRig(t, testIncident("a", "shop", -time.Hour))
	g.s.auth = newServerAuth(Options{Password: testPassword}, g.s.now)
	value, _, err := g.s.auth.sessions.issue()
	require.NoError(t, err)
	srv := httptest.NewServer(g.s.Handler())
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		srv.URL+"/api/events", nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: value})
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	c := &liveClient{cancel: cancel, resp: resp,
		lines: bufio.NewScanner(resp.Body)}
	name, _ := c.next(t)
	require.Equal(t, "ready", name)

	g.s.auth.sessions.revoke(value)
	g.poll <- testEpoch
	name, _ = c.next(t)
	assert.Equal(t, "signedout", name)
	name, _ = c.next(t)
	assert.Empty(t, name, "the stream must end after signedout")
}
