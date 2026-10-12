package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestScoreSpecificityRewardsErrorsNamingTheCause(t *testing.T) {
	secret := inventory.CoreID(kube.KindSecret, "shop", "db-creds")
	t.Run("named", func(t *testing.T) {
		f := newFixture(t)
		usesCase(f, kube.KindSecret, true)
		requireWeight(t, scoreOf(t, f, secret, scoreSpecificity),
			SpecificityWeight)
	})
	t.Run("not named", func(t *testing.T) {
		f := newFixture(t)
		pods := f.workload("shop", "api", 2)
		f.add(secret)
		f.change(secret, 1, "data.password")
		for _, pod := range pods {
			f.relate(pod, inventory.References, secret)
			f.fail(containerOf(pod), "CreateError.Config", failingH, 2,
				"invalid configuration")
		}
		if o := scoreOf(t, f, secret, scoreSpecificity); o.said() {
			t.Fatalf("outcome = %+v, want nothing", o)
		}
	})
}

// A short name is named only as a whole word: "api" is not named by
// "rapid", nor by "api-gateway", whatever else the text says.
func TestScoreSpecificityNeedsAWholeWord(t *testing.T) {
	cm := inventory.CoreID(kube.KindConfigMap, "shop", "api")
	for _, tc := range []struct {
		name, text string
		named      bool
	}{
		{"inside a word", "fatal: rapid reconnect loop", false},
		{"longer object name", "dial api-gateway: refused", false},
		{"quoted", `configmap "api" has an invalid quota`, true},
		{"kind qualified", "failed to read configmap api: bad", true},
		{"host name", "get https://api.example.com: timeout", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			pods := f.workload("shop", "ingest", 2)
			f.add(cm)
			f.change(cm, 1, "data.limits.yaml")
			for _, pod := range pods {
				f.relate(pod, inventory.References, cm)
				f.fail(containerOf(pod), "CrashLoop", failingH, 2, tc.text)
			}
			got := scoreOf(t, f, cm, scoreSpecificity).said()
			if got != tc.named {
				t.Fatalf("named = %v, want %v for %q", got, tc.named,
					tc.text)
			}
		})
	}
}

// An error that quotes a key that changed names the object.
func TestScoreSpecificityRewardsAQuotedChangedKey(t *testing.T) {
	cm := inventory.CoreID(kube.KindConfigMap, "shop", "ab")
	f := newFixture(t)
	pods := f.workload("shop", "ingest", 2)
	f.add(cm)
	f.change(cm, 1, "data.max_connections")
	for _, pod := range pods {
		f.relate(pod, inventory.References, cm)
		f.fail(containerOf(pod), "CrashLoop", failingH, 2,
			"config: max_connections: cannot unmarshal")
	}

	requireWeight(t, scoreOf(t, f, cm, scoreSpecificity), SpecificityWeight)
}
