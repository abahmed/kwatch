package app

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func deniedDynamic(complete bool) fakeDynamic {
	return fakeDynamic{kube.DynamicStatus{
		Complete: complete, Unavailable: 3,
		Reasons: map[string]int{kube.ReasonPermissionDenied: 3},
		Kinds: []kube.DynamicKind{
			{Resource: "certificates.cert-manager.io",
				Reason: kube.ReasonPermissionDenied},
			{Resource: "gateways.gateway.networking.k8s.io",
				Reason: kube.ReasonPermissionDenied},
			{Resource: "rollouts.argoproj.io",
				Reason: kube.ReasonPermissionDenied},
		},
	}}
}

func TestCoverageNoteWarnsOnceAfterFirstCompleteDiscovery(t *testing.T) {
	out := captureKlog(t)
	sink := &fakeStatusSink{}
	source := &fakeAvailability{}
	h := newSourceHealth(sink, source).withCoverage(coveragePublisher{
		sink: &fakeCoverageSink{}, typed: source,
		dynamic: deniedDynamic(false),
	})

	h.report() // discovery not complete yet: nothing to say
	klog.Flush()
	require.NotContains(t, out.String(), "kinds not watched")

	h.coverage.dynamic = deniedDynamic(true)
	h.report()
	h.report()
	klog.Flush()

	text := out.String()
	// klog writes a warning to every lower severity stream as well.
	require.Equal(t, 1, distinctLines(text, "kinds not watched"), text)
	require.Contains(t, text, "3 kinds not watched (permission_denied)")
	require.Contains(t, text, "rollouts.argoproj.io")
	require.Contains(t, text, "set rbac.mode=full to watch custom resources")
	require.True(t, strings.HasPrefix(text, "W"), "must be a warning: "+text)
}

// distinctLines counts the different lines of text containing sub.
func distinctLines(text, sub string) int {
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, sub) {
			seen[line] = true
		}
	}
	return len(seen)
}

func TestCoverageNotePublishesADegradedButAvailableComponent(t *testing.T) {
	_ = captureKlog(t)
	sink := &fakeStatusSink{}
	source := &fakeAvailability{}
	h := newSourceHealth(sink, source).withCoverage(coveragePublisher{
		sink: &fakeCoverageSink{}, typed: source,
		dynamic: deniedDynamic(true),
	})

	h.report()

	got := sink.statuses[coverageComponent]
	require.Equal(t, recordedStatus{
		"degraded", "optional_permission_denied", true}, got,
		"degraded but available: /health stays ok")
}

func TestCoverageNoteIsSilentWhenNothingIsDenied(t *testing.T) {
	out := captureKlog(t)
	sink := &fakeStatusSink{}
	source := &fakeAvailability{}
	h := newSourceHealth(sink, source).withCoverage(coveragePublisher{
		sink: &fakeCoverageSink{}, typed: source,
		dynamic: fakeDynamic{kube.DynamicStatus{Complete: true}},
	})

	h.report()
	klog.Flush()

	require.NotContains(t, out.String(), "kinds not watched")
	_, set := sink.statuses[coverageComponent]
	require.False(t, set)
}

func TestCoverageWarningCapsTheKindList(t *testing.T) {
	var kinds []health.CoverageKind
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h",
		"i", "j"} {
		kinds = append(kinds, health.CoverageKind{
			Kind: name, Reason: health.CoveragePermissionDenied})
	}
	got := coverageWarning(12, kinds)
	require.Contains(t, got, "12 kinds not watched (permission_denied): "+
		"a, b, c, d, e, f, g, h and 4 more;")
}
