package license

import (
	"context"
	"testing"
	"time"
)

func TestManagerBackoffIsJittered(t *testing.T) {
	h := newHarness(t)
	var asked []time.Duration
	h.m.cfg.Jitter = func(max time.Duration) time.Duration {
		asked = append(asked, max)
		return max
	}
	if d := h.m.backoff(); d != time.Minute+12*time.Second {
		t.Fatalf("backoff = %v", d)
	}
	if len(asked) != 1 || asked[0] != 12*time.Second {
		t.Fatalf("jitter bound = %v, want 20%%", asked)
	}
}

func TestManagerInitRaisesClockGuardToLeaseIssueTime(t *testing.T) {
	h := newHarness(t)
	_, priv := testEnv()
	l := testLease(testBinding(), testNow.Add(40*24*time.Hour))
	l.Iat = testNow.Add(30 * 24 * time.Hour).Unix()
	l.SrvTime = l.Iat
	h.leases.lease = SignLease(l, priv) // the clock entry was deleted
	h.m.init(context.Background())
	if got := h.m.clock.MaxSeen; got.Before(time.Unix(l.Iat, 0)) {
		t.Fatalf("guard = %v, want at least the lease iat", got)
	}
}

func TestNewerLeasePrefersAVerifiedLaterLease(t *testing.T) {
	v, priv := testEnv()
	old := testLease(testBinding(), testNow.Add(time.Hour))
	newer := old
	newer.Iat = old.Iat + 60
	a, b := SignLease(old, priv), SignLease(newer, priv)
	if got := NewerLease(v, a, b); got != b {
		t.Fatal("the later lease must win")
	}
	if got := NewerLease(v, "", a); got != a {
		t.Fatal("a lease must win over none")
	}
	if got := NewerLease(v, a, b+"x"); got != a {
		t.Fatal("a tampered lease must never win")
	}
}
