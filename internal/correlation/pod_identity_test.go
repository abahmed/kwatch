package correlation

import (
	"testing"

	"github.com/abahmed/kwatch/internal/event"
)

func TestOwnerlessPodIdentityUsesUID(t *testing.T) {
	first := IncidentKey(event.Event{Resource: "pod", Namespace: "ns", PodName: "worker-a", PodUID: "uid-a", PodGenerateName: "worker-", Reason: "CrashLoopBackOff"}, "worker-a", nil)
	second := IncidentKey(event.Event{Resource: "pod", Namespace: "ns", PodName: "worker-b", PodUID: "uid-b", PodGenerateName: "worker-", Reason: "CrashLoopBackOff"}, "worker-b", nil)
	if first == second {
		t.Fatalf("ownerless Pods with different UIDs must not share a key: %q", first)
	}
}

func TestOwnerlessPodIdentityUsesExplicitLineage(t *testing.T) {
	first := IncidentKey(event.Event{Resource: "pod", Namespace: "ns", PodName: "worker-a", PodUID: "uid-a", PodLineageID: "worker", Reason: "CrashLoopBackOff"}, "worker-a", nil)
	second := IncidentKey(event.Event{Resource: "pod", Namespace: "ns", PodName: "worker-b", PodUID: "uid-b", PodLineageID: "worker", Reason: "CrashLoopBackOff"}, "worker-b", nil)
	if first != second {
		t.Fatalf("ownerless Pods with the same lineage must share a key: %q != %q", first, second)
	}
}

func TestOwnerlessPodGenerateNameDoesNotCreateLineage(t *testing.T) {
	first := IncidentKey(event.Event{Resource: "pod", Namespace: "ns", PodName: "worker-a", PodGenerateName: "worker-", Reason: "CrashLoopBackOff"}, "worker-a", nil)
	second := IncidentKey(event.Event{Resource: "pod", Namespace: "ns", PodName: "worker-b", PodGenerateName: "worker-", Reason: "CrashLoopBackOff"}, "worker-b", nil)
	if first == second {
		t.Fatalf("generateName alone must not merge ownerless Pods: %q", first)
	}
}
