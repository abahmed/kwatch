package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const namedConfigLine = "unexpected end of JSON input reading " +
	"platform/flags key flags.json"

// namedConfigFixture is a ConfigMap of another namespace that an
// application reads through the API, and one replica that crashes on it.
func namedConfigFixture(f *fixture) (inventory.EntityID, inventory.EntityID) {
	config := inventory.CoreID(kube.KindConfigMap, "platform", "flags")
	f.add(config)
	return config, crashOn(f, "shop", "api", namedConfigLine)
}

func TestNamedConfigErrorBlamesTheChangedConfigMap(t *testing.T) {
	f := newFixture(t)
	config, effect := namedConfigFixture(f)
	f.change(config, 1, "data")
	c := requireCause(t, f.explain(), effect, "configmap/platform/flags")
	if c.Row != RuleConfigMapMissingOrChanged {
		t.Fatalf("row = %s, want %s", c.Row, RuleConfigMapMissingOrChanged)
	}
}

func TestNamedConfigErrorIgnoresAConfigMapThatDoesNotExist(t *testing.T) {
	f := newFixture(t)
	effect := crashOn(f, "shop", "api", namedConfigLine)
	if c, ok := f.explain().CauseOf(effect); ok &&
		c.Root.Kind == kube.KindConfigMap {
		t.Fatalf("blamed %s, which does not exist", c.Root)
	}
}

func TestNamedConfigErrorIgnoresAConfigMapThatDidNotChange(t *testing.T) {
	f := newFixture(t)
	_, effect := namedConfigFixture(f)
	if c, ok := f.explain().CauseOf(effect); ok &&
		c.Root.Kind == kube.KindConfigMap {
		t.Fatalf("blamed %s, which did not change", c.Root)
	}
}
