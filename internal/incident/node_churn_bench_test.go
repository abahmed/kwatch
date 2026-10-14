package incident

import (
	"fmt"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// BenchmarkChurnObserve is one look at a cluster of 2000 settled nodes,
// as every Apply and Tick makes.
func BenchmarkChurnObserve(b *testing.B) {
	r := newRig(b, Config{})
	for i := range 2000 {
		node := inventory.CoreID(kube.KindNode, "", fmt.Sprintf("n%d", i))
		r.setAttrs(node, map[string]inventory.Value{
			kube.AttrReady:         inventory.Bool(true),
			kube.AttrUnschedulable: inventory.Bool(false)})
	}
	r.m.churn.observe(r.model, at(0))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r.m.churn.observe(r.model, at(0))
	}
}

// BenchmarkTickNoNodeChange is a Tick on 2000 settled nodes when no node
// changed since the last look: it must not scan them.
func BenchmarkTickNoNodeChange(b *testing.B) {
	r := newRig(b, Config{})
	for i := range 2000 {
		node := inventory.CoreID(kube.KindNode, "", fmt.Sprintf("n%d", i))
		r.setAttrs(node, map[string]inventory.Value{
			kube.AttrReady:         inventory.Bool(true),
			kube.AttrUnschedulable: inventory.Bool(false)})
	}
	r.m.Tick(at(0))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r.m.Tick(at(0))
	}
}
