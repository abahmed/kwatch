package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

// Vectors are produced by the license Worker (kwatch-api, scripts/vectors.ts)
// with a fixed test seed; this proves the Worker's tokens verify here.
type workerVectors struct {
	KID       string `json:"kid"`
	PublicKey string `json:"public_key"`
	Now       int64  `json:"now"`
	Key       struct {
		Token string `json:"token"`
		LID   string `json:"lid"`
		Plan  string `json:"plan"`
		Exp   int64  `json:"exp"`
	} `json:"key"`
	Lease struct {
		Token   string `json:"token"`
		Exp     int64  `json:"exp"`
		Binding struct {
			LID           string `json:"lid"`
			ClusterUUID   string `json:"cluster_uuid"`
			KubeSystemUID string `json:"kube_system_uid"`
			ClusterProof  string `json:"cluster_proof"`
			InstancePub   string `json:"instance_pub"`
		} `json:"binding"`
	} `json:"lease"`
}

func TestWorkerInterop(t *testing.T) {
	raw, err := os.ReadFile("testdata/worker_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vec workerVectors
	if err := json.Unmarshal(raw, &vec); err != nil {
		t.Fatal(err)
	}
	pub, err := base64.RawURLEncoding.DecodeString(vec.PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		t.Fatalf("bad public key: %v", err)
	}
	v := NewVerifier(map[string]ed25519.PublicKey{vec.KID: pub})
	now := time.Unix(vec.Now, 0).UTC()

	key, err := v.VerifyKey(vec.Key.Token, now)
	if err != nil {
		t.Fatalf("VerifyKey: %v", err)
	}
	if key.LID != vec.Key.LID || key.Plan != vec.Key.Plan ||
		key.Exp != vec.Key.Exp {
		t.Errorf("key fields mismatch: %+v", key)
	}
	_, err = v.VerifyKey(vec.Key.Token, time.Unix(vec.Key.Exp, 0))
	if !errors.Is(err, ErrExpired) {
		t.Errorf("key at exp: want ErrExpired, got %v", err)
	}

	b := vec.Lease.Binding
	if got := ClusterProof(b.KubeSystemUID, b.LID); got != b.ClusterProof {
		t.Errorf("cluster proof: Go %s != Worker %s", got, b.ClusterProof)
	}
	want := Binding{
		LID: b.LID, ClusterUUID: b.ClusterUUID,
		ClusterProof: b.ClusterProof, InstancePub: b.InstancePub,
	}
	lease, err := v.VerifyLease(vec.Lease.Token, want, now)
	if err != nil {
		t.Fatalf("VerifyLease: %v", err)
	}
	if lease.InGrace || lease.Exp != vec.Lease.Exp ||
		lease.SrvTime != vec.Now || lease.Plan != PlanPro {
		t.Errorf("lease fields mismatch: %+v", lease)
	}
	// The Worker issues 3 day leases (7 before); either must fit the
	// client's grace so a cluster is never offline beyond ttl+Grace.
	ttl := time.Duration(lease.Exp-lease.Iat) * time.Second
	if ttl < 24*time.Hour || ttl > 7*24*time.Hour {
		t.Errorf("lease ttl %v is outside 1d..7d", ttl)
	}
	if lease.LicExp != vec.Key.Exp {
		t.Errorf("lic_exp mismatch")
	}
	// Grace and hard expiry.
	l, err := v.VerifyLease(vec.Lease.Token, want, time.Unix(vec.Lease.Exp+60, 0))
	if err != nil || !l.InGrace {
		t.Errorf("expected grace, got %v %+v", err, l)
	}
	_, err = v.VerifyLease(vec.Lease.Token, want,
		time.Unix(vec.Lease.Exp, 0).Add(Grace))
	if !errors.Is(err, ErrLeaseExpired) {
		t.Errorf("want ErrLeaseExpired, got %v", err)
	}
	// Wrong binding must be rejected.
	bad := want
	bad.InstancePub = "AAAA"
	_, err = v.VerifyLease(vec.Lease.Token, bad, now)
	if !errors.Is(err, ErrBindingMismatch) {
		t.Errorf("want ErrBindingMismatch, got %v", err)
	}
	// Full pipeline.
	st := Evaluate(Inputs{
		Verifier: v, KeyString: vec.Key.Token, Lease: vec.Lease.Token,
		Binding: want, Now: now,
	})
	if st.State != StateActive {
		t.Errorf("Evaluate state = %s (%s)", st.State, st.Reason)
	}
}
