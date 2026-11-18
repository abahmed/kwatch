package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// uncausedDecision is a crashing Deployment without a cause, whose
// upstream objects of the given kinds were checked, investigated with
// report.
func uncausedDecision(
	report []incident.Section, checked ...inventory.Kind,
) incident.Decision {
	root := inventory.CoreID(kube.KindDeployment, "shop", "api")
	f := detection.Finding{Entity: root,
		Reason: reasons.CrashLoopBackOff, Severity: detection.Critical,
		Since: writerNow.Add(-5 * time.Minute), Summary: "it crashes"}
	p := incident.Incident{ID: "inc-c", Root: root, State: incident.Open,
		Opened: f.Since, Revision: 1, Members: members(f)}
	for _, kind := range checked {
		p.Checked = append(p.Checked, string(kind))
	}
	return incident.Decision{Action: incident.Announce, Incident: p,
		Facts: incident.Facts{Report: report, ChangesRead: true}}
}

var readReport = []incident.Section{
	{Check: incident.CheckPodState, Status: incident.SectionOK},
	{Check: incident.CheckLogs, Status: incident.SectionDenied,
		Reason: incident.ReasonRBAC},
}

func TestEvidenceListsExactlyWhatWasCheckedWithoutACause(t *testing.T) {
	d := uncausedDecision(readReport, kube.KindNode, kube.KindConfigMap,
		explain.KindExternalEndpoint)

	msg := NewWriter("", nil).Write(d, writerNow)

	require.NotNil(t, msg.Evidence)
	assert.Equal(t, []string{
		"kwatch checked: its node is healthy; its ConfigMaps are there " +
			"and did not change; its dependencies show no errors; " +
			"nothing in shop was changed in the last 30 minutes.",
		"kwatch cannot read pod logs here."}, msg.Evidence.Checked)
}

func TestEvidenceSaysNothingWhenNothingWasChecked(t *testing.T) {
	d := uncausedDecision([]incident.Section{
		{Check: incident.CheckPodState, Status: incident.SectionOK}})
	d.Facts.ChangesRead = false

	msg := NewWriter("", nil).Write(d, writerNow)

	require.NotNil(t, msg.Evidence)
	assert.Empty(t, msg.Evidence.Checked)
}

func TestEvidenceRuledOutAlsoForAPossibleCause(t *testing.T) {
	d := uncausedDecision(readReport[:1], kube.KindNode, kube.KindImage)
	db := inventory.CoreID(kube.KindService, "shop", "db")
	d.Incident.Cause = &rootcause.CauseRecord{Root: db, Score: 0.9,
		Rival: &rootcause.CauseRecord{Score: 0.89}}

	msg := NewWriter("", nil).Write(d, writerNow)

	require.NotNil(t, msg.Evidence)
	assert.Equal(t, []string{"Also ruled out: its node and its image."},
		msg.Evidence.Checked)
}

func TestEvidenceKeepsAtMostFiveCheckedItems(t *testing.T) {
	d := uncausedDecision(readReport[:1], kube.KindNode, kube.KindImage,
		kube.KindRegistry, kube.KindSecret, kube.KindPVC,
		kube.KindAccount, kube.KindService)

	msg := NewWriter("", nil).Write(d, writerNow)

	require.Len(t, msg.Evidence.Checked, 1)
	assert.Equal(t, 4, strings.Count(msg.Evidence.Checked[0], "; "))
}

func TestEvidenceStaysQuietWhenChangesWereListed(t *testing.T) {
	d := uncausedDecision(readReport[:1], kube.KindNode)
	d.Facts.Changes = []inventory.Change{{Entity: d.Incident.Root,
		At: writerNow.Add(-time.Minute), Actor: "kubectl-edit"}}

	msg := NewWriter("", nil).Write(d, writerNow)

	assert.Equal(t, []string{"kwatch checked: its node is healthy."},
		msg.Evidence.Checked)
}
