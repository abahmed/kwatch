package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func restartingNote(
	t *testing.T, normal detection.Normality, phrases ...string,
) string {
	t.Helper()
	id := inventory.CoreID(kube.KindContainer, "shop", "api-1/app")
	f := detection.Finding{Entity: id, Reason: reasons.HighRestartCount,
		Severity: detection.Warning, Since: writerNow.Add(-time.Hour),
		Summary: "Container keeps restarting", Normal: normal}
	for _, p := range phrases {
		f.Evidence = append(f.Evidence, detection.Evidence{
			Label: detection.EvidenceBaseline, Value: p})
	}
	p := incident.Incident{ID: "r-1", Root: id, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(f)}
	return Writer{}.Write(announce(p), writerNow).Note
}

func TestBaselineSaysWhenRestartsAreUnusual(t *testing.T) {
	got := restartingNote(t, detection.NormalUnusual,
		"restarts 12×/h vs a usual 0.1×/h")
	assert.Contains(t, got, "That is unusual for this workload: "+
		"restarts 12×/h vs a usual 0.1×/h.")
}

func TestBaselineSaysWhenRestartsAreWithinNormal(t *testing.T) {
	got := restartingNote(t, detection.NormalUsual,
		"restarts 2×/h vs a usual 2×/h")
	assert.Contains(t, got, "That is within what this workload "+
		"normally does: restarts 2×/h vs a usual 2×/h.")
}

func TestBaselineJustQuotesAFigureThatIsNeitherOne(t *testing.T) {
	got := restartingNote(t, detection.NormalUnjudged,
		"restarts 3×/h vs a usual 1×/h")
	assert.Contains(t, got, "Over the last week of this workload: "+
		"restarts 3×/h vs a usual 1×/h.")
}

func TestBaselineIsSilentWithoutHistory(t *testing.T) {
	got := restartingNote(t, detection.NormalUnjudged)
	assert.NotContains(t, got, "workload")
}

func TestDigestShowsTheNormalThatExcusedARestartingWorkload(t *testing.T) {
	id := inventory.CoreID(kube.KindContainer, "shop", "api-1/app")
	f := detection.Finding{Entity: id, Reason: reasons.HighRestartCount,
		Severity: detection.Warning, Since: writerNow.Add(-time.Hour),
		Summary: "Container keeps restarting",
		Normal:  detection.NormalUsual,
		Evidence: []detection.Evidence{{Label: detection.EvidenceBaseline,
			Value: "restarts 2×/h vs a usual 2×/h"}}}
	p := incident.Incident{ID: "r-1", Root: id, Tier: incident.Digest,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(f)}
	got := Writer{}.Digest([]incident.Decision{announce(p)}, nil, nil,
		writerNow)
	assert.Contains(t, got.Note,
		"(within its normal: restarts 2×/h vs a usual 2×/h).")
}
