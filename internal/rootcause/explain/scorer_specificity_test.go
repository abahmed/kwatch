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
