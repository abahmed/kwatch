package pipeline

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// gatedPod is a pod held by a scheduling gate since since.
func gatedPod(name string, since time.Time) *corev1.Pod {
	p := pod(name, "queue-rs", "", false, since)
	p.Status.Phase = corev1.PodPending
	p.Status.Conditions = []corev1.PodCondition{{
		Type: corev1.PodScheduled, Status: corev1.ConditionFalse,
		Reason:             "SchedulingGated",
		LastTransitionTime: metav1.NewTime(since),
	}}
	return p
}

// withheldHarness is a cold-started edition harness whose detectors
// count what the edition withholds.
func withheldHarness(
	t *testing.T, start time.Time, e feature.Entitlements, hide bool,
) (*harness, *feature.Withheld) {
	t.Helper()
	withheld := feature.NewWithheld()
	h := newHarnessWith(t, start, func(d *Dependencies) {
		d.Entitlements, d.Withheld = e, withheld
		d.Detectors = detection.NewRegistry(nil,
			detectors.Entitled(e, withheld)...)
		d.Writer = compose.Writer{}.ForEdition(e, hide)
	})
	h.engine.announcer.collect.Startup.ColdStart = true
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	h.add(kube.PodSchema{}, gatedPod("queue-a", start.Add(-time.Hour)))
	h.engine.reconcileDowntime()
	return h, withheld
}

func proChecksNotices(h *harness) []string {
	var out []string
	for _, m := range h.messages {
		if m.IsNotice() && strings.Contains(m.Note, "Pro checks") {
			out = append(out, m.Note)
		}
	}
	return out
}

func TestCommunityCountsProFindingsWithoutAlerting(t *testing.T) {
	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	h, withheld := withheldHarness(t, start,
		feature.CommunityEntitlements(), false)

	h.run(start.Add(5*time.Minute), 10*time.Second)

	assert.Equal(t, 1, withheld.Counts(h.now)[feature.WithheldGates])
	assert.Empty(t, h.engine.deps.Incidents.Incidents(),
		"a withheld finding opens no incident")
	notices := proChecksNotices(h)
	require.Len(t, notices, 1, "%v", h.messages)
	assert.Contains(t, notices[0], "1 scheduling gate")
	assert.NotContains(t, notices[0], "queue-a", "no names")
	report := h.engine.Status()
	require.NotNil(t, report.ProChecks)
	assert.Equal(t, 1, report.ProChecks.Total())
}

func TestHideProHintsDropsTheCountNoticeNotStatus(t *testing.T) {
	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	h, _ := withheldHarness(t, start, feature.CommunityEntitlements(), true)

	h.run(start.Add(5*time.Minute), 10*time.Second)

	assert.Empty(t, proChecksNotices(h))
	report := h.engine.Status()
	require.NotNil(t, report.ProChecks, "/status still counts")
	assert.Equal(t, 1, report.ProChecks.Total())
}

func TestProAlertsOnWhatCommunityWithholds(t *testing.T) {
	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	h, withheld := withheldHarness(t, start, feature.ProEntitlements(),
		false)

	h.run(start.Add(5*time.Minute), 10*time.Second)

	assert.Zero(t, withheld.Counts(h.now).Total())
	assert.Empty(t, proChecksNotices(h))
	assert.Nil(t, h.engine.Status().ProChecks)
	assert.NotEmpty(t, h.engine.deps.Incidents.Incidents(),
		"pro raises the gated pod")
}
