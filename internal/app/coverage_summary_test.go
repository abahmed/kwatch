package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

type fakeCoverageSink struct{ got []health.CoverageSummary }

func (f *fakeCoverageSink) SetCoverage(s health.CoverageSummary) {
	f.got = append(f.got, s)
}

type fakeDynamic struct{ status kube.DynamicStatus }

func (f fakeDynamic) Status() kube.DynamicStatus { return f.status }

func TestBuildCoverageMergesTypedAndDynamic(t *testing.T) {
	typed := []kube.SourceStatus{
		{Resource: "secrets", Reason: kube.ReasonPermissionDenied},
		{Resource: "ingresses", Group: "networking.k8s.io",
			Reason: kube.ReasonSyncTimeout},
		{Resource: "nodes", Reason: kube.ReasonSyncFailed},
	}
	dynamic := kube.DynamicStatus{
		Watched:     map[kube.WatchMode]int{kube.WatchStatus: 7},
		Skipped:     4,
		Unavailable: 2,
		Complete:    true,
	}

	got := buildCoverage(typed, dynamic)

	require.Equal(t, 7, got.Watched["status"])
	require.Equal(t, 4, got.Skipped)
	require.Equal(t, 5, got.Unavailable)
	require.True(t, got.Complete)
	require.Equal(t, []health.CoverageKind{
		{Kind: "ingresses.networking.k8s.io",
			Reason: "cache_sync_timeout"},
		{Kind: "nodes", Reason: "cache_sync_failed"},
		{Kind: "secrets", Reason: "permission_denied"},
	}, got.UnavailableKinds)
	require.Equal(t, 1, got.Watched["hashed"], "secrets is down")
	require.Positive(t, got.Watched["full"])
}

func TestBuildCoverageCapsKindList(t *testing.T) {
	var typed []kube.SourceStatus
	for i := 0; i < health.MaxCoverageKinds+5; i++ {
		typed = append(typed, kube.SourceStatus{
			Resource: "r" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Reason:   kube.ReasonAPIUnavailable,
		})
	}

	got := buildCoverage(typed, kube.DynamicStatus{})

	require.Len(t, got.UnavailableKinds, health.MaxCoverageKinds)
	require.True(t, got.Truncated)
	require.Equal(t, len(typed), got.Unavailable)
}

func TestSourceHealthReportPublishesCoverage(t *testing.T) {
	sink := &fakeStatusSink{}
	source := &fakeAvailability{}
	coverage := &fakeCoverageSink{}
	h := newSourceHealth(sink, source).withCoverage(coveragePublisher{
		sink: coverage, typed: source,
		dynamic: fakeDynamic{kube.DynamicStatus{Skipped: 1}},
	})

	h.report()

	require.Len(t, coverage.got, 1)
	require.Equal(t, 1, coverage.got[0].Skipped)
}

func TestCoveragePublisherIgnoresMissingDependencies(t *testing.T) {
	coveragePublisher{}.publish()
}

// disabledAvailability is a source with Secrets turned off by config.
type disabledAvailability struct{ fakeAvailability }

func (*disabledAvailability) Disabled() []kube.SourceStatus {
	return []kube.SourceStatus{{
		Resource: "secrets", Reason: kube.ReasonDisabledByConfig,
	}}
}

func TestCoverageListsSecretsDisabledByConfig(t *testing.T) {
	sink := &fakeStatusSink{}
	source := &disabledAvailability{}
	coverage := &fakeCoverageSink{}
	h := newSourceHealth(sink, source).withCoverage(coveragePublisher{
		sink: coverage, typed: source, dynamic: fakeDynamic{},
	})

	h.report()

	require.Len(t, coverage.got, 1)
	require.Equal(t, []health.CoverageKind{{
		Kind: "secrets", Reason: health.CoverageDisabledByConfig,
	}}, coverage.got[0].UnavailableKinds)
	require.Zero(t, coverage.got[0].Watched["hashed"]-1,
		"only ConfigMaps are watched hashed")
}

// Every source code keeps its own name: a watch kwatch could not sync
// is never reported as a kind the API does not serve.
func TestCoverageReasonMapsEverySourceCode(t *testing.T) {
	for _, code := range []string{
		kube.ReasonPermissionDenied, kube.ReasonAPIUnavailable,
		kube.ReasonSyncFailed, kube.ReasonSyncTimeout,
		kube.ReasonSyncPending, kube.ReasonHandlerPanics,
		kube.ReasonDisabledByConfig, kube.ReasonWatchBudget,
		kube.ReasonObjectCap,
	} {
		require.Equal(t, code, coverageReason(code))
	}
	require.Equal(t, health.CoverageUnknown, coverageReason("raw: x"))
}

// Before a discovery ran there is nothing partial to report, as in
// the dashboard's coverage.
func TestBuildCoverageCompleteBeforeDiscoveryRan(t *testing.T) {
	cases := []struct {
		name   string
		status kube.DynamicStatus
		want   bool
	}{
		{"never ran", kube.DynamicStatus{}, true},
		{"ran and complete",
			kube.DynamicStatus{Ran: true, Complete: true}, true},
		{"ran and partial", kube.DynamicStatus{Ran: true}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildCoverage(nil, c.status)
			require.Equal(t, c.want, got.Complete)
			require.Equal(t, c.status.Ran, got.Ran)
		})
	}
}
