package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// An object named after a common word is not named by prose that holds
// the word: a ConfigMap called "config" is not what "failed to load
// config" points at.
func TestScoreSpecificityIgnoresGenericObjectNames(t *testing.T) {
	for _, tc := range []struct {
		name, object string
		named        bool
	}{
		{"config", "config", false},
		{"default", "default", false},
		{"data", "data", false},
		{"compound name", "api-config", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cm := inventory.CoreID(kube.KindConfigMap, "shop", tc.object)
			f := newFixture(t)
			pods := f.workload("shop", "ingest", 2)
			f.add(cm)
			f.change(cm, 1, "data.limits.yaml")
			for _, pod := range pods {
				f.relate(pod, inventory.References, cm)
				f.fail(containerOf(pod), "CrashLoop", failingH, 2,
					"fatal: failed to load config api-config: bad data "+
						"in default")
			}
			got := scoreOf(t, f, cm, scoreSpecificity).said()
			if got != tc.named {
				t.Fatalf("named = %v, want %v", got, tc.named)
			}
		})
	}
}

// limitRangeFixture is a namespace with two LimitRanges and a controller
// whose creates are refused with text.
func limitRangeFixture(
	f *fixture, text string,
) (first, second, effect inventory.EntityID) {
	first = inventory.CoreID(kube.KindLimitRange, "shop", "team-bounds")
	second = inventory.CoreID(kube.KindLimitRange, "shop", "batch-bounds")
	for _, lr := range []inventory.EntityID{first, second} {
		f.add(lr)
		f.relate(lr, inventory.Constrains,
			inventory.CoreID(kube.KindNamespace, "", "shop"))
	}
	return first, second, createCase(f, text)
}

// TestLimitRangeIsToldApartByTheNameTheErrorQuotes: when the refusal
// names one of the namespace's LimitRanges, the others are not blamed.
func TestLimitRangeIsToldApartByTheNameTheErrorQuotes(t *testing.T) {
	f := newFixture(t)
	first, second, effect := limitRangeFixture(f,
		"Error creating: pods \"api-1-x\" is forbidden: maximum cpu "+
			"usage per Container is 1, but limit is 2 (limitrange "+
			"team-bounds)")
	v := newView(f.snapshot())

	if len(v.limitRangeModes(first, effect, LinkAdmits)) == 0 {
		t.Fatal("the named LimitRange was not blamed")
	}
	if got := v.limitRangeModes(second, effect, LinkAdmits); len(got) > 0 {
		t.Fatalf("an unnamed LimitRange was blamed: %v", got)
	}
}

// TestLimitRangesStayRivalsWhenTheErrorNamesNone: with no name in the
// refusal both LimitRanges remain suspects.
func TestLimitRangesStayRivalsWhenTheErrorNamesNone(t *testing.T) {
	f := newFixture(t)
	first, second, effect := limitRangeFixture(f, limitRangeText)
	v := newView(f.snapshot())

	for _, lr := range []inventory.EntityID{first, second} {
		if len(v.limitRangeModes(lr, effect, LinkAdmits)) == 0 {
			t.Fatalf("%s was dropped without a name to tell it apart", lr)
		}
	}
}
