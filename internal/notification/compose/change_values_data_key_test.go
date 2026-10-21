package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A config key's field only says whether the key was added, changed or
// removed; "Key app.yaml changed to changed." repeats the word.
func TestChangeValuesLeaveOutTheStateOfAConfigKey(t *testing.T) {
	for _, state := range []string{"added", "changed", "removed"} {
		change := inventory.Change{
			Entity: inventory.CoreID(kube.KindConfigMap, "web", "cfg"),
			Fields: []inventory.FieldChange{{
				Path: "data.app.yaml", After: state}},
		}

		assert.Empty(t, changeValues(change), state)
		assert.Empty(t, setValue(change), state)
	}
}

func TestChangeValuesStillSayWhatAnUnsetLabelBecame(t *testing.T) {
	change := inventory.Change{
		Entity: inventory.CoreID(kube.KindNamespace, "", "web"),
		Fields: []inventory.FieldChange{{
			Path: "labels.pod-security", After: "enforce=restricted"}},
	}

	assert.Equal(t, " to enforce=restricted", changeValues(change))
}

// A hidden value reads "changed" on both sides; saying "from changed to
// changed" tells the reader nothing.
func TestChangeValuesLeaveOutHiddenValues(t *testing.T) {
	change := inventory.Change{
		Entity: inventory.CoreID(kube.KindDeployment, "web", "api"),
		Fields: []inventory.FieldChange{{
			Path:   "spec.template.spec.containers[0].env.FOO",
			Before: "changed", After: "changed"}},
	}

	assert.Empty(t, changeValues(change))
}
