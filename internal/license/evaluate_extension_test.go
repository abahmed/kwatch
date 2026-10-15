package license

import (
	"strings"
	"testing"
	"time"
)

func TestEvaluateEndIsLaterOfKeyAndLicExp(t *testing.T) {
	v, priv := testEnv()
	b := testBinding()
	keyExp := testNow.Add(90 * 24 * time.Hour)
	l := testLease(b, testNow.Add(24*time.Hour))
	l.LicExp = keyExp.Add(100 * 24 * time.Hour).Unix()
	st := Evaluate(Inputs{Verifier: v, KeyString: SignKey(testKey(keyExp), priv),
		Lease: SignLease(l, priv), Binding: b, Now: testNow})
	if !st.ExpiresAt.Equal(time.Unix(l.LicExp, 0)) {
		t.Fatalf("ExpiresAt = %v, want lic_exp", st.ExpiresAt)
	}
	l.LicExp = testNow.Add(24 * time.Hour).Unix() // shorter than the key
	st = Evaluate(Inputs{Verifier: v, KeyString: SignKey(testKey(keyExp), priv),
		Lease: SignLease(l, priv), Binding: b, Now: testNow})
	if !st.ExpiresAt.Equal(keyExp) {
		t.Fatalf("ExpiresAt = %v, want key exp", st.ExpiresAt)
	}
}

func TestEvaluateStatusFields(t *testing.T) {
	v, priv := testEnv()
	b := testBinding()
	keyExp := testNow.Add(90 * 24 * time.Hour)
	leaseExp := testNow.Add(24 * time.Hour)
	l := testLease(b, leaseExp)
	l.LicExp = keyExp.Unix()
	st := Evaluate(Inputs{
		Verifier:  v,
		KeyString: SignKey(testKey(keyExp), priv),
		Lease:     SignLease(l, priv),
		Binding:   b,
		Now:       testNow,
	})
	if st.State != StateActive || st.LID != "lic_…1234" ||
		strings.Contains(st.LID, "abcdef") {
		t.Fatalf("status = %+v", st)
	}
	if !st.ExpiresAt.Equal(keyExp) || !st.GraceUntil.Equal(leaseExp.Add(Grace)) {
		t.Fatalf("times = %v %v", st.ExpiresAt, st.GraceUntil)
	}
}
