package kube

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCoverageSplitsNamespacesByScope(t *testing.T) {
	got := BuildCoverage(CoverageInput{
		Namespaces: []string{"shop", "kube-system", "default"},
		InScope:    func(ns string) bool { return ns != "kube-system" },
	})

	assert.Equal(t, 2, got.Namespaces.Watched)
	assert.Equal(t, []string{"default", "shop"}, got.Namespaces.WatchedNames)
	assert.Equal(t, []string{"kube-system"}, got.Namespaces.ExcludedNames)
	assert.False(t, got.Namespaces.All)
}

func TestBuildCoverageBoundsTheNamespaceNames(t *testing.T) {
	var namespaces []string
	for i := range MaxCoverageNames + 5 {
		namespaces = append(namespaces, fmt.Sprintf("ns-%03d", i))
	}

	got := BuildCoverage(CoverageInput{Namespaces: namespaces, All: true})

	assert.Equal(t, MaxCoverageNames+5, got.Namespaces.Watched)
	assert.Len(t, got.Namespaces.WatchedNames, MaxCoverageNames)
	assert.True(t, got.Namespaces.Truncated)
	assert.Empty(t, got.Namespaces.ExcludedNames)
}

func TestBuildCoverageCountsWatchedAndSyncedKinds(t *testing.T) {
	synced := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := BuildCoverage(CoverageInput{
		Typed: []WatchedKind{
			{Kind: "pods", Mode: "full", Synced: true},
			{Kind: "secrets", Mode: "hashed"},
		},
		Dynamic: DynamicStatus{
			Watched: map[WatchMode]int{WatchStatus: 3}, Synced: 2,
			Ran: true, Complete: true,
		},
		SyncedAt: synced,
	})

	assert.Equal(t, 5, got.Kinds.Watched)
	assert.Equal(t, 3, got.Kinds.Synced)
	assert.Equal(t, map[string]int{"full": 1, "hashed": 1, "status": 3},
		got.Kinds.ByMode)
	assert.Equal(t, synced, got.SyncedAt)
	assert.True(t, got.DiscoveryComplete)
}

// A typed watch that lost access counts once, as unreadable, not also
// as watched.
func TestBuildCoverageCountsARefusedWatchOnce(t *testing.T) {
	got := BuildCoverage(CoverageInput{
		Typed: []WatchedKind{
			{Kind: "pods", Mode: "full", Synced: true},
			{Kind: "secrets", Mode: "hashed"},
		},
		Unavailable: []SourceStatus{
			{Resource: "secrets", Reason: ReasonPermissionDenied},
		},
	})

	assert.Equal(t, 1, got.Kinds.Watched)
	assert.Equal(t, map[string]int{"full": 1}, got.Kinds.ByMode)
	require.Len(t, got.Kinds.Unreadable, 1)
	assert.Equal(t, "secrets", got.Kinds.Unreadable[0].Kind)
}

func TestBuildCoverageExplainsKindsItCannotRead(t *testing.T) {
	got := BuildCoverage(CoverageInput{
		Unavailable: []SourceStatus{
			{Resource: "secrets", Reason: ReasonPermissionDenied},
			{Resource: "leases", Group: "coordination.k8s.io",
				Reason: ReasonSyncPending, Required: true},
		},
		Disabled: []SourceStatus{{Resource: "configmaps",
			Reason: ReasonDisabledByConfig}},
		Dynamic: DynamicStatus{
			Kinds: []DynamicKind{{Resource: "widgets.example.com",
				Reason: ReasonWatchBudget}},
			Reasons: map[string]int{ReasonWatchBudget: 7},
		},
	})

	require.Len(t, got.Kinds.Unreadable, 1)
	assert.Equal(t, "secrets", got.Kinds.Unreadable[0].Kind)
	assert.Contains(t, got.Kinds.Unreadable[0].Detail, "RBAC")
	require.Len(t, got.Kinds.Unsynced, 1)
	assert.Equal(t, "leases.coordination.k8s.io", got.Kinds.Unsynced[0].Kind)
	assert.True(t, got.Kinds.Unsynced[0].Required)
	assert.Len(t, got.Kinds.NotWatched, 2)
	assert.Equal(t, 7, got.Kinds.Reasons[ReasonWatchBudget],
		"counts include what the bounded lists leave out")
}

func TestBuildCoverageBoundsEachGapList(t *testing.T) {
	var denied []SourceStatus
	for i := range MaxCoverageNames + 1 {
		denied = append(denied, SourceStatus{
			Resource: fmt.Sprintf("r%03d", i), Reason: ReasonPermissionDenied,
		})
	}

	got := BuildCoverage(CoverageInput{Unavailable: denied})

	assert.Len(t, got.Kinds.Unreadable, MaxCoverageNames)
	assert.True(t, got.Kinds.Truncated)
	assert.Equal(t, MaxCoverageNames+1,
		got.Kinds.Reasons[ReasonPermissionDenied])
}

func TestGapDetailOfAnUnknownReasonIsTheAPIOne(t *testing.T) {
	assert.Equal(t, gapDetails[ReasonAPIUnavailable], gapDetail("other"))
}

func TestBuildCoverageDiscoveryCompleteOnlyWhenARunWasPartial(t *testing.T) {
	cases := []struct {
		name   string
		status DynamicStatus
		want   bool
	}{
		{"never ran or off", DynamicStatus{}, true},
		{"ran and complete", DynamicStatus{Ran: true, Complete: true}, true},
		{"ran and partial", DynamicStatus{Ran: true}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := BuildCoverage(CoverageInput{Dynamic: c.status})
			assert.Equal(t, c.want, got.DiscoveryComplete)
		})
	}
}
