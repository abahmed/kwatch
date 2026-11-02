package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestRadiusSummary(t *testing.T) {
	payments := entity(kube.KindDeployment, "payments")
	cases := []struct {
		name string
		r    Radius
		want string
	}{
		{"nothing", Radius{}, ""},
		{"pods of one workload",
			Radius{Services: 2, Ingresses: 1, Pods: 3, Of: payments,
				Replicas: 4},
			"Affects 2 services, 1 ingress and 3 of 4 pods of payments"},
		{"pods of several workloads",
			Radius{Workloads: 2, Pods: 5}, "Affects 2 workloads and 5 pods"},
		{"more pods than replicas",
			Radius{Pods: 3, Of: payments, Replicas: 1}, "Affects 3 pods"},
		{"one pod of one replica",
			Radius{Pods: 1, Of: payments, Replicas: 1},
			"Affects 1 of 1 pod of payments"},
		{"one service", Radius{Services: 1}, "Affects 1 service"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.r.Summary())
		})
	}
}

func TestRadiusKeepsTheWidest(t *testing.T) {
	r := newRig(t, Config{})
	deploy, pod := r.workloadRig(t, 3, 1, 4)
	other := entity(kube.KindPod, "api-2")
	r.relate(other, inventory.OwnedBy, deploy)
	r.cause(pod, deploy, "api crashes")
	r.cause(other, deploy, "api crashes")
	r.raise(at(0), crashSig(pod), crashSig(other))
	p := r.only()
	assert.Equal(t, Radius{Pods: 2, Of: deploy, Replicas: 3}, p.Radius)
	assert.Equal(t, p.Radius, p.Journal.Widest)

	r.clear(at(time.Minute), crashSig(other))
	p = r.only()
	assert.Equal(t, 2, p.Journal.Widest.Pods, "the widest stays")
	assert.Equal(t, 2, r.m.Export()[0].Widest.Pods, "and is persisted")
}

// A wider radius with fewer pods must not lower the pod peak, which
// "N pods were affected" reads.
func TestRadiusKeepsThePodPeakWhenTheRadiusWidens(t *testing.T) {
	svc := func(n string) inventory.EntityID {
		return entity(kube.KindService, n)
	}
	p := &Incident{Root: svc("a"),
		Impact:  []inventory.EntityID{svc("b"), svc("c"), svc("d")},
		Journal: Journal{Widest: Radius{Pods: 3}}}

	p.noteRadius(inventory.NewModel(inventory.Options{}))

	assert.Equal(t, 4, p.Journal.Widest.Services, "the wider radius")
	assert.Equal(t, 3, p.Journal.Widest.Pods, "the pod peak stays")
}
