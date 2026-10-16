package license

import (
	"testing"
	"time"
)

func TestEvaluate(t *testing.T) {
	v, priv := testEnv()
	b := testBinding()
	keyExp := testNow.Add(90 * 24 * time.Hour)
	key := SignKey(testKey(keyExp), priv)
	leaseExp := testNow.Add(24 * time.Hour)
	lease := SignLease(testLease(b, leaseExp), priv)
	wrongBind := b
	wrongBind.ClusterProof = ClusterProof("other", b.LID)
	expiredKey := SignKey(testKey(testNow.Add(-time.Hour)), priv)
	noExt := testLease(b, leaseExp)
	noExt.LicExp = testNow.Add(-time.Hour).Unix()
	future := testLease(b, leaseExp)
	future.Nbf = testNow.Add(time.Hour).Unix()

	base := Inputs{Verifier: v, KeyString: key, Lease: lease,
		Binding: b, Now: testNow}
	with := func(f func(*Inputs)) Inputs { in := base; f(&in); return in }

	tests := []struct {
		name  string
		in    Inputs
		state State
		pro   bool
	}{
		{"no key", with(func(i *Inputs) { i.KeyString = "" }), StateCommunity, false},
		{"garbage key", with(func(i *Inputs) {
			i.KeyString = "nope"
		}), StateInvalid, false},
		{"forged key", with(func(i *Inputs) {
			_, o := testPair("attacker")
			i.KeyString = SignKey(testKey(keyExp), o)
		}), StateInvalid, false},
		{"expired key", with(func(i *Inputs) {
			i.KeyString, i.Lease = expiredKey, ""
		}), StateExpired, false},
		{"expired key, lease without extension", with(func(i *Inputs) {
			i.KeyString, i.Lease = expiredKey, SignLease(noExt, priv)
		}), StateExpired, false},
		{"expired key beats revoked", with(func(i *Inputs) {
			i.KeyString, i.ServerVerdict, i.Lease = expiredKey, VerdictRevoked, ""
		}), StateExpired, false},
		{"expired key extended by lease", with(func(i *Inputs) {
			i.KeyString = expiredKey
		}), StateActive, true},
		{"expired key extended, lease in grace", with(func(i *Inputs) {
			i.KeyString, i.Now = expiredKey, leaseExp.Add(time.Hour)
		}), StateGrace, true},
		{"expired key, lease past grace", with(func(i *Inputs) {
			i.KeyString, i.Now = expiredKey, leaseExp.Add(Grace)
		}), StateExpired, false},
		{"expired key, extension ended", with(func(i *Inputs) {
			i.KeyString, i.Now = expiredKey, testNow.Add(366*24*time.Hour)
		}), StateExpired, false},
		{"expired key, extension on foreign lease", with(func(i *Inputs) {
			i.KeyString, i.Binding = expiredKey, wrongBind
		}), StateExpired, false},
		{"expired key, forged lease", with(func(i *Inputs) {
			_, o := testPair("attacker")
			i.KeyString, i.Lease = expiredKey, SignLease(testLease(b, leaseExp), o)
		}), StateExpired, false},
		{"forged key not extended", with(func(i *Inputs) {
			_, o := testPair("attacker")
			i.KeyString = SignKey(testKey(testNow.Add(-time.Hour)), o)
		}), StateInvalid, false},
		{"key expiry via clock guard", with(func(i *Inputs) {
			i.Now = testNow.Add(-48 * time.Hour)
			i.Clock = ClockGuard{MaxSeen: keyExp.Add(time.Hour)}
		}), StateExpired, false},
		{"revoked", with(func(i *Inputs) {
			i.ServerVerdict = VerdictRevoked
		}), StateRevoked, false},
		{"conflict", with(func(i *Inputs) {
			i.ServerVerdict = VerdictConflict
		}), StateConflict, false},
		{"no lease", with(func(i *Inputs) { i.Lease = "" }), StatePending, false},
		{"lease not yet valid", with(func(i *Inputs) {
			i.Lease = SignLease(future, priv)
		}), StatePending, false},
		{"lease wrong cluster", with(func(i *Inputs) {
			i.Binding = wrongBind
		}), StateInvalid, false},
		{"lease garbage", with(func(i *Inputs) {
			i.Lease = "KWL1-x.y"
		}), StateInvalid, false},
		{"lease for other license", with(func(i *Inputs) {
			ob := b
			ob.LID = "lic_other"
			i.Lease = SignLease(testLease(ob, leaseExp), priv)
		}), StateInvalid, false},
		{"active", base, StateActive, true},
		{"active with clock ahead guard", with(func(i *Inputs) {
			i.Clock = ClockGuard{MaxSeen: testNow.Add(-time.Hour)}
		}), StateActive, true},
		{"grace after lease exp", with(func(i *Inputs) {
			i.Now = leaseExp.Add(time.Hour)
		}), StateGrace, true},
		{"grace end", with(func(i *Inputs) {
			i.Now = leaseExp.Add(Grace)
		}), StateExpired, false},
		{"past grace", with(func(i *Inputs) {
			i.Now = leaseExp.Add(Grace + time.Hour)
		}), StateExpired, false},
		{"clock rolled back", with(func(i *Inputs) {
			i.Clock = ClockGuard{MaxSeen: testNow.Add(2 * time.Hour)}
		}), StateGrace, true},
		{"clock rolled back past grace", with(func(i *Inputs) {
			i.Now = testNow.Add(-72 * time.Hour)
			i.Clock = ClockGuard{MaxSeen: leaseExp.Add(Grace + time.Hour)}
		}), StateExpired, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := Evaluate(tt.in)
			if st.State != tt.state {
				t.Fatalf("state = %s (%s), want %s", st.State, st.Reason, tt.state)
			}
			if st.State.ProEnabled() != tt.pro {
				t.Fatalf("ProEnabled = %v", !tt.pro)
			}
			if st.State != StateCommunity && st.State != StateInvalid &&
				st.Plan != PlanPro {
				t.Fatalf("plan = %q", st.Plan)
			}
		})
	}
}

func TestEvaluateNilVerifierUsesDefault(t *testing.T) {
	_, priv := testEnv()
	st := Evaluate(Inputs{
		KeyString: SignKey(testKey(testNow.Add(time.Hour)), priv),
		Now:       testNow,
	})
	if st.State != StateInvalid {
		t.Fatalf("state = %s", st.State)
	}
}
