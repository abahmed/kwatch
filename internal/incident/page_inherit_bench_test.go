package incident

import (
	"fmt"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// BenchmarkInheritPageStorm is a new incident arriving while 2000 others
// are live: the cost of looking for a paged story of the same workload.
func BenchmarkInheritPageStorm(b *testing.B) {
	r := newRig(b, Config{})
	for i := range 2000 {
		pod := entity(kube.KindPod, fmt.Sprintf("app%d-0", i))
		r.raise(at(0), sig(pod, "CrashLoopBackOff", detection.Warning))
	}
	fresh := sig(entity(kube.KindPod, "newcomer-0"),
		"CrashLoopBackOff", detection.Warning)
	b.ResetTimer()
	for range b.N {
		p := &Incident{State: Settling}
		r.m.inheritPage(at(0), p, fresh)
	}
}
