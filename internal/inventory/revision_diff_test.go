package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func edit(path, before, after string) FieldChange {
	return FieldChange{Path: path, Before: before, After: after}
}

func TestRankFieldsPutsLikeliestCulpritFirst(t *testing.T) {
	ranked := RankFields([]FieldChange{
		edit("volumes[cfg]", "a", "b"),
		edit("containers[x].readinessProbe", "", "tcp"),
		edit("containers[x].resources.limits.memory", "512Mi", "256Mi"),
		edit("containers[x].args", "", "--fast"),
		edit("containers[x].env.MODE", "a", "b"),
		edit("containers[x].image", "x:1", "x:2"),
	})

	var paths []string
	for _, f := range ranked {
		paths = append(paths, f.Path)
	}
	assert.Equal(t, []string{
		"containers[x].image", "containers[x].env.MODE",
		"containers[x].args", "containers[x].resources.limits.memory",
		"containers[x].readinessProbe", "volumes[cfg]",
	}, paths)
}

func TestRankFieldsKeepsOrderOfEqualRanks(t *testing.T) {
	ranked := RankFields([]FieldChange{
		edit("spec.b", "", ""), edit("spec.a", "", "")})

	assert.Equal(t, "spec.b", ranked[0].Path)
}

func TestRevisionDiffFoldsSuccessiveEdits(t *testing.T) {
	changes := []Change{
		{Fields: []FieldChange{edit("containers[x].image", "x:1", "x:2"),
			edit("spec.replicas", "2", "3")}},
		{Fields: []FieldChange{edit("containers[x].image", "x:2", "x:3"),
			edit("containers[x].env.A", "1", "2")}},
		{Fields: []FieldChange{edit("containers[x].env.A", "2", "1")}},
	}

	got := RevisionDiff(changes)

	assert.Equal(t, []FieldChange{
		edit("containers[x].image", "x:1", "x:3")}, got,
		"first old value, last new value; edited back and replicas drop")
}

func TestRevisionDiffIsEmptyWithoutTemplateEdits(t *testing.T) {
	assert.Empty(t, RevisionDiff([]Change{{Fields: []FieldChange{
		edit("spec.replicas", "2", "3")}}}))
}
