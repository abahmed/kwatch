package dynamicwatch

import (
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

func TestWatcherStatusReportsUnavailableWithoutResources(t *testing.T) {
	var watcher Watcher

	got := watcher.Status()
	if got.State != "unavailable" {
		t.Fatalf("Status().State = %q, want unavailable", got.State)
	}
}

func TestWatcherTracksOptionalAvailabilityTransitions(t *testing.T) {
	gvr := schema.GroupVersionResource{
		Group: "example.io", Version: "v1", Resource: "widgets",
	}
	watcher := &Watcher{}

	if got := watcher.updateUnavailableLocked([]schema.GroupVersionResource{
		gvr,
	}); got != 1 {
		t.Fatalf("first unavailable state reported %d transitions", got)
	}
	if got := watcher.updateUnavailableLocked([]schema.GroupVersionResource{
		gvr,
	}); got != 0 {
		t.Fatalf("repeated unavailable state reported %d transitions", got)
	}
	if got := watcher.updateUnavailableLocked(nil); got != 0 {
		t.Fatalf("recovery reported %d transitions", got)
	}
	if got := watcher.updateUnavailableLocked([]schema.GroupVersionResource{
		gvr,
	}); got != 1 {
		t.Fatalf("second outage reported %d transitions", got)
	}
}

func TestWatcherStatusReportsSkippedResourcesAsDegraded(t *testing.T) {
	watcher := Watcher{
		skippedGVR: []schema.GroupVersionResource{
			{Group: "example.io", Version: "v1", Resource: "widgets"},
		},
	}

	got := watcher.Status()
	if got.State != "degraded" || got.Skipped != 1 {
		t.Fatalf("Status() = %+v, want one degraded resource", got)
	}
	if len(got.SkippedResources) != 1 ||
		got.SkippedResources[0] != "example.io/v1, Resource=widgets" {
		t.Fatalf("Status().SkippedResources = %v", got.SkippedResources)
	}
}

func TestWatcherStatusReportsUnsyncedInformerAsPartial(t *testing.T) {
	watcher := Watcher{
		informers: []cache.SharedIndexInformer{
			cache.NewSharedIndexInformer(
				&cache.ListWatch{},
				&runtime.Unknown{},
				0,
				cache.Indexers{},
			),
		},
	}

	got := watcher.Status()
	if got.State != "partial" || got.Unsynced != 1 {
		t.Fatalf("Status() = %+v, want one partial informer", got)
	}
}

func TestWatcherStatusJSONUsesStableFieldNames(t *testing.T) {
	payload, err := json.Marshal(Status{
		State: "degraded", InformerCount: 2, Skipped: 1,
	})
	if err != nil {
		t.Fatalf("Marshal(Status) returned error: %v", err)
	}
	const want = `{"state":"degraded","informerCount":2,` +
		`"synced":0,"unsynced":0,"skipped":1}`
	if string(payload) != want {
		t.Fatalf("Marshal(Status) = %s, want %s", payload, want)
	}
}
