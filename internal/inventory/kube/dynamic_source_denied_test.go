package kube

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func gvrOf(resource string) schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group: "example.com", Version: "v1", Resource: resource,
	}
}

func TestDynamicSourceSummarisesRefusedTypesSorted(t *testing.T) {
	src := &DynamicSource{refused: map[schema.GroupVersionResource]*refusal{
		gvrOf("widgets"): {kindIssue: kindIssue{"widget",
			ReasonPermissionDenied}},
		gvrOf("alphas"): {kindIssue: kindIssue{"alpha",
			ReasonPermissionDenied}},
		gvrOf("betas"): {kindIssue: kindIssue{"beta",
			ReasonAPIUnavailable}},
	}}

	types, denied := src.refusedSummaryLocked()

	assert.Equal(t, "alphas.example.com,betas.example.com,"+
		"widgets.example.com", types)
	assert.Equal(t, 2, denied)
}

func TestDynamicSourceSummaryBoundsTheTypeList(t *testing.T) {
	src := &DynamicSource{refused: map[schema.GroupVersionResource]*refusal{}}
	for i := 0; i < maxLoggedRefused+5; i++ {
		src.refused[gvrOf(fmt.Sprintf("r%02d", i))] = &refusal{
			kindIssue: kindIssue{"k", ReasonPermissionDenied}}
	}

	types, _ := src.refusedSummaryLocked()

	assert.Contains(t, types, ",+5", "the rest is counted, not listed")
}

func TestDynamicSourceLogsRefusedTypesOnlyWhenTheyChange(t *testing.T) {
	src := &DynamicSource{}

	first := src.refusedChanged("a,b")
	again := src.refusedChanged("a,b")
	grown := src.refusedChanged("a,b,c")
	emptied := src.refusedChanged("")
	back := src.refusedChanged("a,b,c")

	assert.True(t, first)
	assert.False(t, again, "an unchanged list is not logged at every pass")
	assert.True(t, grown)
	assert.False(t, emptied, "an empty list is never logged")
	assert.True(t, back)
}

// passes returns, for the passes after the n-th permission refusal of
// gvr, how many retry passes are skipped before the next try.
func skippedPasses(src *DynamicSource, gvr schema.GroupVersionResource) int {
	skipped := 0
	for !src.denialDue(gvr) {
		skipped++
		if skipped > 100 {
			return skipped
		}
	}
	return skipped
}

func TestDynamicSourceBacksOffTypesRefusedForPermission(t *testing.T) {
	src := &DynamicSource{}
	gvr := gvrOf("widgets")
	var got []int
	for refusal := 1; refusal <= 6; refusal++ {
		src.noteRefusal(gvr, ReasonPermissionDenied)
		got = append(got, skippedPasses(src, gvr))
	}

	// Try at the next pass, then after 1, 3 and at most 7 skipped passes.
	assert.Equal(t, []int{0, 1, 3, 7, 7, 7}, got)
}

func TestDynamicSourceOtherRefusalsAreRetriedEveryPass(t *testing.T) {
	src := &DynamicSource{}
	gvr := gvrOf("widgets")
	src.noteRefusal(gvr, ReasonPermissionDenied)
	src.noteRefusal(gvr, ReasonPermissionDenied)
	src.noteRefusal(gvr, ReasonAPIUnavailable) // not a permission problem

	assert.Zero(t, skippedPasses(src, gvr))
	assert.Empty(t, src.denials)
}
