package dashboard

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
)

// A peer inside the trusted range that rotates the client it names is
// cut off by the proxy's hard limit, and a flood of new client keys
// never locks out a client the limiter has not seen.
func TestRotatedForwardedClientsAreBoundedAndDoNotLockOthersOut(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	a := NewAuth("tok", "RIGHTPASSWORD1234", true,
		clock.Func(func() time.Time { return now }))
	a.trusted, _ = parseProxies([]string{"10.0.0.0/8"})
	login := func(peer, xff, pw string) int {
		r := httptest.NewRequest(http.MethodPost, "/login",
			strings.NewReader(`{"password":"`+pw+`"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Forwarded-Proto", "https")
		r.Header.Set("X-Forwarded-For", xff)
		r.RemoteAddr = peer + ":4444"
		w := httptest.NewRecorder()
		a.loginHandler(w, r)
		return w.Code
	}
	wrong := 0
	for i := range 20000 {
		ip := fmt.Sprintf("203.0.%d.%d", i>>8&255, i&255)
		if login("10.9.9.9", ip, "wrong") == http.StatusUnauthorized {
			wrong++
		}
	}
	if wrong > maxProxyHard {
		t.Fatalf("%d wrong passwords through one peer in a minute, "+
			"want at most %d", wrong, maxProxyHard)
	}
	if got := login("10.1.1.1", "198.51.100.7",
		"RIGHTPASSWORD1234"); got != http.StatusNoContent {
		t.Fatalf("a real user through another proxy got %d", got)
	}
}

// Past the key bound the oldest key is forgotten: a new key starts with
// no failures instead of sharing a full overflow bucket.
func TestLimiterEvictsTheOldestKey(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	r := newRateLimiter(clock.Func(func() time.Time { return now }), 1)
	for i := range maxLimiterKeys + 5 {
		r.record(fmt.Sprintf("k%d", i))
	}
	if !r.allowed("fresh") {
		t.Fatal("an unseen key is locked out")
	}
	if len(r.counts) > maxLimiterKeys || len(r.order) > maxLimiterKeys {
		t.Fatalf("bound broken: %d counts, %d keys", len(r.counts),
			len(r.order))
	}
}

// IPv6 clients count under their /64; addresses of other families and
// IPv4 count as themselves.
func TestLimitKey(t *testing.T) {
	for in, want := range map[string]string{
		"2001:db8:1:2:aaaa::1": "2001:db8:1:2::/64",
		"2001:db8:1:2:bbbb::9": "2001:db8:1:2::/64",
		"203.0.113.5":          "203.0.113.5",
		"::ffff:203.0.113.5":   "::ffff:203.0.113.5",
		"not-an-address":       "not-an-address",
	} {
		if got := limitKey(in); got != want {
			t.Errorf("limitKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// Wide ranges are accepted with a warning (warnBroadProxy); only an
// entry that is not an address or a range is refused.
func TestParseProxiesKeepsWideRanges(t *testing.T) {
	set, errs := parseProxies([]string{"0.0.0.0/0", "10.0.0.0/8",
		"10.1.2.3", "nope"})
	if len(errs) != 1 || len(set) != 3 {
		t.Fatalf("set %v, errs %v", set, errs)
	}
}

// A generated grouped password is accepted in any case and without its
// dashes; a chosen password must match exactly.
func TestPasswordNormalizedOnlyInTheGeneratedForm(t *testing.T) {
	now := clock.Func(func() time.Time { return time.Unix(0, 0) })
	gen := NewAuth("tok", "XM86-CS8S-LB77-F3GE", false, now)
	for _, typed := range []string{"XM86-CS8S-LB77-F3GE",
		"xm86cs8slb77f3ge", "xm86 cs8s lb77 f3ge"} {
		if !gen.validPassword(typed) {
			t.Errorf("generated password refused as %q", typed)
		}
	}
	chosen := NewAuth("tok", "Summer-2026x", false, now)
	if !chosen.validPassword("Summer-2026x") {
		t.Error("chosen password refused")
	}
	for _, typed := range []string{"SUMMER2026X", "summer-2026x"} {
		if chosen.validPassword(typed) {
			t.Errorf("chosen password accepted as %q", typed)
		}
	}
}
