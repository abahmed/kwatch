package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// classCase fails two pods of shop/api, which read the changed object,
// with text.
func classCase(
	t *testing.T, kind inventory.Kind, field, text string,
) (*fixture, inventory.EntityID) {
	f := newFixture(t)
	id := inventory.CoreID(kind, "shop", "obj")
	f.add(id)
	if kind == kube.KindNetworkPolicy {
		f.change(id, 1)
	} else {
		f.change(id, 1, field)
	}
	for _, pod := range f.workload("shop", "api", 2) {
		if kind == kube.KindNetworkPolicy {
			f.links[id] = append(f.links[id],
				Link{Type: inventory.Selects, To: pod})
		} else {
			f.relate(pod, inventory.References, id)
		}
		f.fail(containerOf(pod), "CrashLoop", failingH, 2, text)
	}
	return f, id
}

func TestScoreErrorClassFitsTheKindOfObject(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kind  inventory.Kind
		field string
		text  string
		want  float64
	}{
		{"parse error fits config", kube.KindConfigMap, "data.a.yaml",
			"panic: strconv.ParseInt: parsing \"0.14\": invalid syntax",
			ErrorClassWeight},
		{"timeout is not a config's", kube.KindConfigMap, "data.mode",
			"dial tcp 10.0.3.4:5432: i/o timeout", -ErrorClassPenalty},
		{"timeout fits a policy", kube.KindNetworkPolicy, "",
			"dial tcp 10.0.3.4:5432: i/o timeout", ErrorClassWeight},
		{"timeout may follow a changed address", kube.KindConfigMap,
			"data.DB_HOST", "dial tcp 10.0.3.4:5432: i/o timeout", 0},
		{"nil pointer says nothing of a config", kube.KindConfigMap,
			"data.mode", "panic: runtime error: nil pointer dereference",
			0},
		{"unclassified error", kube.KindConfigMap, "data.mode",
			"fatal: cannot start", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, id := classCase(t, tc.kind, tc.field, tc.text)

			got := scoreOf(t, f, id, scoreErrorClass)

			if tc.want == 0 {
				if got.said() {
					t.Fatalf("outcome = %+v, want nothing", got)
				}
				return
			}
			requireWeight(t, got, tc.want)
		})
	}
}

func TestScoreErrorClassNeedsHalfOfTheFailures(t *testing.T) {
	f := newFixture(t)
	id := inventory.CoreID(kube.KindConfigMap, "shop", "obj")
	f.add(id)
	f.change(id, 1, "data.mode")
	pods := f.workload("shop", "api", 4)
	for i, pod := range pods {
		text := "fatal: cannot start"
		if i == 0 {
			text = "dial tcp 10.0.3.4:5432: i/o timeout"
		}
		f.relate(pod, inventory.References, id)
		f.fail(containerOf(pod), "CrashLoop", failingH, 2, text)
	}

	if o := scoreOf(t, f, id, scoreErrorClass); o.said() {
		t.Fatalf("outcome = %+v, want nothing", o)
	}
}
