package kube

import (
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestSourceVerifiableSeparatesVirtualFromUnsyncedKinds(t *testing.T) {
	s := &Source{synced: map[inventory.Kind]cache.InformerSynced{
		KindPod:    func() bool { return true },
		KindSecret: func() bool { return false },
	}}

	cases := []struct {
		name string
		kind inventory.Kind
		want bool
	}{
		{"synced watched kind", KindPod, true},
		{"unsynced watched kind", KindSecret, false},
		{"container follows its pod", KindContainer, true},
		{"virtual kind is derived", inventory.Kind("registry"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.Verifiable(tc.kind); got != tc.want {
				t.Fatalf("Verifiable(%s) = %v, want %v",
					tc.kind, got, tc.want)
			}
		})
	}
}

// fakeDynamicKinds serves fixed dynamic kind states.
type fakeDynamicKinds map[inventory.Kind]KindState

func (f fakeDynamicKinds) KindState(kind inventory.Kind) (KindState, bool) {
	state, ok := f[kind]
	return state, ok
}

func TestSourceAsksDynamicKindsForUntypedKinds(t *testing.T) {
	s := &Source{
		synced: map[inventory.Kind]cache.InformerSynced{
			KindPod: func() bool { return true },
		},
		cfg: SourceConfig{Dynamic: fakeDynamicKinds{
			"gateway":  {Synced: true},
			"widget":   {Reason: ReasonPermissionDenied},
			"podgroup": {Reason: ReasonWatchBudget},
		}},
	}

	cases := []struct {
		kind               inventory.Kind
		synced, verifiable bool
	}{
		{KindPod, true, true},
		{"gateway", true, true},
		{"widget", false, false},
		{"podgroup", false, false},
		{"registry", false, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			if got := s.Synced(tc.kind); got != tc.synced {
				t.Fatalf("Synced = %v, want %v", got, tc.synced)
			}
			if got := s.Verifiable(tc.kind); got != tc.verifiable {
				t.Fatalf("Verifiable = %v, want %v", got, tc.verifiable)
			}
		})
	}
}

// A Role revoked after the initial sync leaves the cache full of old
// objects; the kind must stop being verifiable (so incidents on it do
// not resolve from stale data) until a relist gets through again.
func TestSourceVerifiableAfterAccessLostUntilRelist(t *testing.T) {
	version := "100"
	configMaps := &watchState{
		resource:    Resource{Name: "configmaps"},
		hasSynced:   func() bool { return true },
		lastVersion: func() string { return version },
	}
	s := &Source{
		synced: map[inventory.Kind]cache.InformerSynced{
			KindConfigMap: configMaps.hasSynced,
		},
		watches: []*watchState{configMaps},
	}
	forbidden := apierrors.NewForbidden(
		schema.GroupResource{Resource: "configmaps"}, "", nil)
	if !s.Verifiable(KindConfigMap) {
		t.Fatal("a synced kind is verifiable")
	}

	configMaps.record(forbidden)
	if s.Verifiable(KindConfigMap) {
		t.Fatal("access lost after sync must make the kind unverifiable")
	}
	unavailable := s.Unavailable()
	if len(unavailable) != 1 ||
		unavailable[0].Reason != ReasonPermissionDenied {
		t.Fatalf("Unavailable = %+v, want configmaps permission_denied",
			unavailable)
	}
	// Retries keep failing: still unverifiable.
	configMaps.record(forbidden)
	if s.Verifiable(KindConfigMap) {
		t.Fatal("a failed retry does not restore access")
	}

	version = "180" // a relist succeeded
	if !s.Verifiable(KindConfigMap) {
		t.Fatal("a successful relist restores the kind")
	}
	if got := s.Unavailable(); len(got) != 0 {
		t.Fatalf("Unavailable = %+v, want none", got)
	}
}

// A refusal before the first sync is an ordinary unsynced kind.
func TestSourceAccessRefusedBeforeSyncIsNotLost(t *testing.T) {
	w := &watchState{
		resource:    Resource{Name: "configmaps"},
		hasSynced:   func() bool { return false },
		lastVersion: func() string { return "" },
	}
	w.record(apierrors.NewUnauthorized("expired token"))
	if w.accessLost() {
		t.Fatal("never synced, so nothing was lost")
	}
}
