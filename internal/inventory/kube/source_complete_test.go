package kube

import (
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/inventory"
)

// A Role revoked mid-run freezes the kind's cache: Synced still says
// true, but an object created afterwards never reaches the model, so
// the kind must not count as complete for "it does not exist" until a
// relist gets through.
func TestSourceCompleteFalseWhileAccessLost(t *testing.T) {
	version := "100"
	secrets := &watchState{
		resource:    Resource{Name: "secrets"},
		hasSynced:   func() bool { return true },
		lastVersion: func() string { return version },
	}
	s := &Source{
		synced: map[inventory.Kind]cache.InformerSynced{
			KindSecret: secrets.hasSynced,
		},
		watches: []*watchState{secrets},
	}
	if !s.Complete(KindSecret) {
		t.Fatal("a synced kind is complete")
	}

	secrets.record(apierrors.NewForbidden(
		schema.GroupResource{Resource: "secrets"}, "", nil))
	if !s.Synced(KindSecret) {
		t.Fatal("the initial sync stays done")
	}
	if s.Complete(KindSecret) {
		t.Fatal("a kind whose access was lost must not be complete")
	}

	version = "180" // a relist succeeded
	if !s.Complete(KindSecret) {
		t.Fatal("a successful relist makes the kind complete again")
	}
}

// Complete never widens Synced: a kind no source watches is verifiable
// (derived) but not synced, so it is not complete either.
func TestSourceCompleteNeedsSynced(t *testing.T) {
	s := &Source{synced: map[inventory.Kind]cache.InformerSynced{
		KindPod: func() bool { return false },
	}}
	for _, kind := range []inventory.Kind{KindPod, "registry"} {
		if s.Complete(kind) {
			t.Fatalf("Complete(%s) = true, want false", kind)
		}
	}
}
