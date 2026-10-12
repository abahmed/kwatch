package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The parts of the root are the root failing, not the failure spreading
// to something else.
func TestSpreadSentencesSkipPartsOfTheRoot(t *testing.T) {
	pod := inventory.CoreID(kube.KindPod, "shop", "startup-error")
	deploy := inventory.CoreID(kube.KindDeployment, "shop", "api")
	other := inventory.CoreID(kube.KindDeployment, "shop", "api-gateway")
	svc := inventory.CoreID(kube.KindService, "shop", "cart")
	container := func(pod, name string) inventory.EntityID {
		return inventory.CoreID(kube.KindContainer, "shop", pod+"/"+name)
	}
	tests := []struct {
		name string
		root inventory.EntityID
		part inventory.EntityID
		want int
	}{
		{"container of the root pod", pod,
			container("startup-error", "workload"), 0},
		{"pod of the root workload", deploy,
			inventory.CoreID(kube.KindPod, "shop", "api-7d9f-a"), 0},
		{"container of a root workload pod", deploy,
			container("api-7d9f-a", "app"), 0},
		{"pod of another workload", deploy,
			inventory.CoreID(kube.KindPod, "shop", "api-gateway-1-a"), 1},
		{"container of another pod", pod,
			container("other-pod", "workload"), 1},
		{"service", deploy, svc, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := badRollout()
			p.Root = tt.root
			p.Impact = []inventory.EntityID{tt.root, other}
			finding := detection.Finding{Entity: tt.part,
				Reason: "CrashLooping", Summary: "keeps crashing"}

			got := spreadSentences(caseFacts{p: incident.Incident(p)},
				[]detection.Finding{finding})

			if len(got) != tt.want {
				t.Fatalf("got %d sentences %+v, want %d", len(got), got,
					tt.want)
			}
		})
	}
}
