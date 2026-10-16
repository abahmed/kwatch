package license

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func testPair(seed string) (ed25519.PublicKey, ed25519.PrivateKey) {
	s := sha256.Sum256([]byte(seed))
	priv := ed25519.NewKeyFromSeed(s[:])
	return priv.Public().(ed25519.PublicKey), priv
}

func testEnv() (*Verifier, ed25519.PrivateKey) {
	pub, priv := testPair("signer")
	return NewVerifier(map[string]ed25519.PublicKey{"v1": pub}), priv
}

func testKey(exp time.Time) Key {
	return Key{V: 1, KID: "v1", LID: "lic_abcdef1234", Acc: "acc_1",
		Plan: PlanPro, Iat: testNow.Add(-time.Hour).Unix(), Exp: exp.Unix(), Max: 1}
}

func testBinding() Binding {
	return Binding{
		LID:          "lic_abcdef1234",
		ClusterUUID:  "uuid-1",
		ClusterProof: ClusterProof("kube-uid", "lic_abcdef1234"),
		InstancePub:  "aW5zdGFuY2U=",
	}
}

func testLease(b Binding, exp time.Time) Lease {
	return Lease{KID: "v1", LID: b.LID, ClusterUUID: b.ClusterUUID,
		ClusterProof: b.ClusterProof, InstancePub: b.InstancePub, Plan: PlanPro,
		LicExp: testNow.Add(365 * 24 * time.Hour).Unix(),
		Iat:    testNow.Add(-time.Hour).Unix(), Nbf: testNow.Add(-time.Hour).Unix(),
		Exp: exp.Unix(), SrvTime: testNow.Unix()}
}

func TestVerifyKey(t *testing.T) {
	v, priv := testEnv()
	_, other := testPair("attacker")
	good := SignKey(testKey(testNow.Add(24*time.Hour)), priv)
	unknown := testKey(testNow.Add(time.Hour))
	unknown.KID = "v9"
	badPlan := testKey(testNow.Add(time.Hour))
	badPlan.Plan = "gold"
	badVer := testKey(testNow.Add(time.Hour))
	badVer.V = 2

	tests := []struct {
		name string
		in   string
		want error
	}{
		{"valid", good, nil},
		{"valid with whitespace", " \n" + good + "\n", nil},
		{"forged signature",
			SignKey(testKey(testNow.Add(time.Hour)), other), ErrBadSignature},
		{"tampered payload", tamper(good), ErrBadSignature},
		{"unknown kid", SignKey(unknown, priv), ErrUnknownKID},
		{"expired", SignKey(testKey(testNow.Add(-time.Second)), priv), ErrExpired},
		{"expires exactly now", SignKey(testKey(testNow), priv), ErrExpired},
		{"empty", "", ErrMalformed},
		{"no prefix", strings.TrimPrefix(good, "KW1-"), ErrMalformed},
		{"no dot", "KW1-abcd", ErrMalformed},
		{"bad base64", "KW1-!!!.???", ErrMalformed},
		{"bad json",
			"KW1-" + enc("nope") + "." + enc(strings.Repeat("x", 64)),
			ErrMalformed},
		{"bad plan", SignKey(badPlan, priv), ErrMalformed},
		{"bad version", SignKey(badVer, priv), ErrMalformed},
		{"lease prefix", "KWL1-a.b", ErrMalformed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, err := v.VerifyKey(tt.in, testNow)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if tt.want == nil && k.LID != "lic_abcdef1234" {
				t.Fatalf("lid = %q", k.LID)
			}
		})
	}
}

func TestParseKey(t *testing.T) {
	_, priv := testEnv()
	k, err := ParseKey(" " + SignKey(testKey(testNow.Add(time.Hour)), priv) + " ")
	if err != nil || k.Plan != PlanPro || k.Max != 1 {
		t.Fatalf("k = %+v, err = %v", k, err)
	}
	if _, err := ParseKey("garbage"); !errors.Is(err, ErrMalformed) {
		t.Fatalf("err = %v", err)
	}
}

func TestDefaultVerifierRejectsKeysNotSignedByProduction(t *testing.T) {
	_, priv := testEnv()
	_, err := DefaultVerifier().VerifyKey(
		SignKey(testKey(testNow.Add(time.Hour)), priv), testNow,
	)
	if !errors.Is(err, ErrBadSignature) &&
		!errors.Is(err, ErrUnknownKID) {
		t.Fatalf("err = %v", err)
	}
}

func TestVerifyLease(t *testing.T) {
	v, priv := testEnv()
	_, other := testPair("attacker")
	b := testBinding()
	exp := testNow.Add(24 * time.Hour)
	good := SignLease(testLease(b, exp), priv)

	wrongProof := b
	wrongProof.ClusterProof = ClusterProof("other-uid", b.LID)
	wrongInst := b
	wrongInst.InstancePub = "b3RoZXI="
	wrongUUID := b
	wrongUUID.ClusterUUID = "uuid-2"
	wrongLID := b
	wrongLID.LID = "lic_zzzz"
	future := testLease(b, exp)
	future.Nbf = testNow.Add(time.Hour).Unix()

	tests := []struct {
		name      string
		lease     string
		binding   Binding
		now       time.Time
		want      error
		wantGrace bool
	}{
		{"valid", good, b, testNow, nil, false},
		{"forged",
			SignLease(testLease(b, exp), other), b, testNow, ErrBadSignature,
			false},
		{"tampered", tamper(good), b, testNow, ErrBadSignature, false},
		{"wrong proof", good, wrongProof, testNow, ErrBindingMismatch, false},
		{"wrong instance", good, wrongInst, testNow, ErrBindingMismatch, false},
		{"wrong uuid", good, wrongUUID, testNow, ErrBindingMismatch, false},
		{"wrong lid", good, wrongLID, testNow, ErrBindingMismatch, false},
		{"not yet valid",
			SignLease(future, priv), b, testNow, ErrLeaseNotYetValid, false},
		{"at exp is grace", good, b, exp, nil, true},
		{"inside grace", good, b, exp.Add(Grace - time.Second), nil, true},
		{"grace end", good, b, exp.Add(Grace), ErrLeaseExpired, false},
		{"past grace", good, b, exp.Add(Grace + time.Hour), ErrLeaseExpired, false},
		{"malformed", "KWL1-x", b, testNow, ErrMalformed, false},
		{"key token", "KW1-a.b", b, testNow, ErrMalformed, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := v.VerifyLease(tt.lease, tt.binding, tt.now)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if err == nil && l.InGrace != tt.wantGrace {
				t.Fatalf("InGrace = %v, want %v", l.InGrace, tt.wantGrace)
			}
		})
	}
}

func TestUnknownLeaseKID(t *testing.T) {
	v, priv := testEnv()
	l := testLease(testBinding(), testNow.Add(time.Hour))
	l.KID = "v7"
	_, err := v.VerifyLease(SignLease(l, priv), testBinding(), testNow)
	if !errors.Is(err, ErrUnknownKID) {
		t.Fatalf("err = %v", err)
	}
}

func TestClusterProof(t *testing.T) {
	got := ClusterProof("uid", "lic_1")
	if len(got) != 64 {
		t.Fatalf("got %q", got)
	}
	if got != ClusterProof("uid", "lic_1") ||
		got == ClusterProof("uid2", "lic_1") ||
		got == ClusterProof("uid", "lic_2") {
		t.Fatal("proof must be deterministic and depend on both inputs")
	}
	sum := sha256.Sum256([]byte("uid:lic_1"))
	if got[:8] != hex8(sum[:4]) {
		t.Fatalf("not sha256(uid:lid): %q", got)
	}
}

func hex8(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, 8)
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&15])
	}
	return string(out)
}

func TestClockGuard(t *testing.T) {
	var g ClockGuard
	if g.RolledBack(testNow) {
		t.Fatal("zero guard must not report rollback")
	}
	g.Observe(testNow)
	g.Observe(testNow.Add(-time.Hour * 5))
	if !g.MaxSeen.Equal(testNow) {
		t.Fatalf("MaxSeen = %v", g.MaxSeen)
	}
	tests := []struct {
		name string
		now  time.Time
		back bool
		eff  time.Time
	}{
		{"later", testNow.Add(time.Minute), false, testNow.Add(time.Minute)},
		{"within tolerance", testNow.Add(-time.Hour), false, testNow},
		{"beyond tolerance", testNow.Add(-time.Hour - time.Second), true, testNow},
		{"day back", testNow.Add(-24 * time.Hour), true, testNow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if g.RolledBack(tt.now) != tt.back {
				t.Fatalf("RolledBack = %v", !tt.back)
			}
			if !g.Effective(tt.now).Equal(tt.eff) {
				t.Fatalf("Effective = %v", g.Effective(tt.now))
			}
		})
	}
}

func TestMaskLID(t *testing.T) {
	tests := map[string]string{
		"lic_abcdef1234": "lic_…1234",
		"ab":             "lic_…",
		"":               "lic_…",
	}
	for in, want := range tests {
		if got := MaskLID(in); got != want {
			t.Errorf("MaskLID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProEnabled(t *testing.T) {
	for _, s := range []State{StateCommunity, StateInvalid, StatePending,
		StateExpired, StateRevoked, StateConflict} {
		if s.ProEnabled() {
			t.Errorf("%s must not enable Pro", s)
		}
	}
	if !StateActive.ProEnabled() || !StateGrace.ProEnabled() {
		t.Error("active and grace must enable Pro")
	}
}

// tamper swaps in a modified, still valid payload but keeps the signature.
func tamper(tok string) string {
	i := strings.Index(tok, "-")
	dot := strings.LastIndex(tok, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(tok[i+1 : dot])
	changed := strings.Replace(string(raw), `"pro"`, `"trial"`, 1)
	changed = strings.Replace(changed, `"lic_abcdef1234"`, `"lic_abcdef9999"`, 1)
	return tok[:i+1] +
		base64.RawURLEncoding.EncodeToString([]byte(changed)) +
		tok[dot:]
}

func enc(s string) string {
	return strings.TrimRight(base64URL([]byte(s)), "=")
}

func base64URL(b []byte) string { return base64.URLEncoding.EncodeToString(b) }
