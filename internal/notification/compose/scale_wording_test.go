package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func scaleChange(before, after string) inventory.Change {
	return inventory.Change{
		Entity: inventory.CoreID(kube.KindDeployment, "shop", "api"),
		At:     time.Date(2026, 1, 5, 10, 4, 0, 0, time.UTC),
		Fields: []inventory.FieldChange{{Path: replicasPath,
			Before: before, After: after}},
	}
}

// A change of the replica count alone is a scale, not a release.
func TestScaleIsNotWordedAsARelease(t *testing.T) {
	up, down := scaleChange("2", "5"), scaleChange("5", "2")
	assert.True(t, scaleOnly(up))
	assert.Equal(t, "scale-up", scaleNoun(up))
	assert.Equal(t, "scale-down", scaleNoun(down))
	assert.Equal(t, "scaling", scaleNoun(scaleChange("", "2")))
	assert.Equal(t, "the replicas were scaled from 2 to 5 at 10:04",
		scaledText(up))
	assert.Equal(t, "the 10:04 scale-up",
		changePhrase(up, up.Entity))

	rollout := up
	rollout.Fields = append(rollout.Fields, inventory.FieldChange{
		Path: "spec.template", Before: "a", After: "b"})
	assert.False(t, scaleOnly(rollout), "a template edit is a release")
	assert.Equal(t, "the 10:04 release", changePhrase(rollout,
		rollout.Entity))
}
