package dashboard

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
)

// redactNone leaves text as it is, for tests that do not redact.
func redactNone(s string) string { return s }

// One client can hold only maxClientLive streams, so it cannot take
// every slot; other clients still get one, and a closed stream gives
// its slot back.
func TestLiveHubLimitsStreamsPerClient(t *testing.T) {
	h := newLiveHub()
	for range maxClientLive {
		assert.Equal(t, liveOK, h.acquire("session a"))
	}
	assert.Equal(t, liveClientFull, h.acquire("session a"))
	assert.Equal(t, liveOK, h.acquire("session b"))

	h.release("session a")
	assert.Equal(t, liveOK, h.acquire("session a"))
}

// The per-client refusal is 429 with Retry-After; the client is the
// address when it signed in with the bearer token.
func TestLiveRefusesAClientPastItsStreams(t *testing.T) {
	g := newLiveRig(t)
	for range maxClientLive {
		assert.Equal(t, liveOK, g.s.live.acquire("address 192.0.2.1"))
	}

	w := get(t, g.s, "/api/events")

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "30", w.Header().Get("Retry-After"))
}

// The live version fingerprints rows as the API shows them, redacted:
// two incidents that differ only in a redacted secret look the same,
// so the version says nothing about the secret.
func TestLiveRevsFingerprintRedactedText(t *testing.T) {
	a := testIncident("a", "shop", 0)
	a.Headline = "login fails: password=hunter2 rejected"
	b := a
	b.Headline = "login fails: password=swordfish rejected"
	var texts textCache

	redacted := liveRevs([]incident.Incident{a}, texts.redact)
	other := liveRevs([]incident.Incident{b}, texts.redact)

	assert.Equal(t, redacted["a"], other["a"])
	assert.NotEqual(t, liveRevs([]incident.Incident{a}, redactNone)["a"],
		liveRevs([]incident.Incident{b}, redactNone)["a"])
}

// A made-up session cookie does not name a client: the stream counts
// against the address, so rotating cookies buys no new allowance.
func TestLiveClientIgnoresAJunkCookie(t *testing.T) {
	s := newSessionServer(false, testPassword).Server
	req := cookieRequest("junk")
	assert.Equal(t, "address 192.0.2.1", s.liveClient(req))

	value, _, err := s.auth.sessions.issue()
	assert.NoError(t, err)
	assert.Equal(t, "session "+value, s.liveClient(cookieRequest(value)))
}
