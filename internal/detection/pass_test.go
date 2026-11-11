package detection

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// passDetector builds one shared value per pass and counts the builds.
type passDetector struct{ builds *int }

func (passDetector) Name() string            { return "pass" }
func (passDetector) Kinds() []inventory.Kind { return []inventory.Kind{"Pod"} }
func (d passDetector) Detect(ctx Context, _ inventory.Entity) []Finding {
	ctx.Pass("shared", func() any { *d.builds++; return 1 })
	return nil
}

func TestPassValueIsBuiltOncePerPass(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	model := inventory.NewModel(inventory.Options{})
	builds := 0
	r := NewRegistry(nil, passDetector{builds: &builds})
	for _, name := range []string{"a", "b", "c"} {
		id := inventory.EntityID{Kind: "Pod", Namespace: "ns", Name: name}
		_, _ = model.Apply(inventory.Observation{Kind: inventory.Observed,
			Source: "test", At: now, Entity: id})
		r.Evaluate(model, now, id)
	}
	if builds != 1 {
		t.Fatalf("built %d times in one pass, want 1", builds)
	}
	r.Evaluate(model, now.Add(time.Second), inventory.EntityID{Kind: "Pod",
		Namespace: "ns", Name: "a"})
	if builds != 2 {
		t.Fatalf("built %d times over two passes, want 2", builds)
	}
}

func TestContextPassWithoutRegistryBuildsEveryTime(t *testing.T) {
	n := 0
	ctx := Context{}
	for i := 0; i < 2; i++ {
		ctx.Pass("k", func() any { n++; return n })
	}
	if n != 2 {
		t.Fatalf("builds = %d, want 2", n)
	}
}

// A second evaluation at the same instant against another model must
// not reuse the value the first one built.
func TestPassValueIsNotSharedAcrossModels(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	builds := 0
	r := NewRegistry(nil, passDetector{builds: &builds})
	id := inventory.EntityID{Kind: "Pod", Namespace: "ns", Name: "a"}
	for range 2 {
		model := inventory.NewModel(inventory.Options{})
		_, _ = model.Apply(inventory.Observation{Kind: inventory.Observed,
			Source: "test", At: now, Entity: id})
		r.Evaluate(model, now, id)
	}
	if builds != 2 {
		t.Fatalf("built %d times for two models, want 2", builds)
	}
}
