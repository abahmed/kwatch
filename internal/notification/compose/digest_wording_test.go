package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// A title whose first "is" sits in a later clause keeps its own verb:
// cutting there would put a dash inside the sentence.
func TestItemLineKeepsASentenceWhoseVerbComesFirst(t *testing.T) {
	title := "Two objects in arc-system have been stuck deleting since " +
		"Dec 4, 2025: their finalizer a.io/cleanup is never removed."
	assert.Equal(t, strings.TrimSuffix(title, "."),
		itemLine(title, "arc-system"))
	assert.Equal(t, "Service web (shop) — failing",
		itemLine("Service web in shop is failing.", "shop"))
}

func TestProseNameShortensGeneratedNames(t *testing.T) {
	hash := "csi-5468c63f" + strings.Repeat("0", 50) + "9d2e1a"
	assert.Equal(t, "csi-5468c63f…9d2e1a", proseName(hash))
	long := strings.Repeat("a", maxProseName+1)
	assert.Equal(t, "aaaaaaaaaaaa…aaaaaa", proseName(long))
	pod := "actions-runner-controller-github-webhook-server-5f7d8c9b6-x"
	assert.Equal(t, pod, proseName(pod), "an ordinary name stays whole")
	assert.Equal(t, "volume attachment csi-5468c63f…9d2e1a", shortName(
		inventory.CoreID(kube.KindVolumeAttachment, "", hash)))
}

// Objects with no namespace are not placed: no "of the cluster".
func TestFinalizerLeadDoesNotPlaceClusterScopedObjects(t *testing.T) {
	f := heldFacts(unhandledCause(explain.ModeFinalizerUnhandled), "", 2,
		"widgets.example.com/cleanup")
	got := leadSentences(f)[0].text
	assert.True(t, strings.HasPrefix(got, "Two objects (secret old-0 and "+
		"secret old-1) (prod-eu-1) have been stuck deleting since Oct 3"),
		got)
	f.cluster = ""
	got = leadSentences(f)[0].text
	assert.True(t, strings.HasPrefix(got, "Two objects (secret old-0 and "+
		"secret old-1) have been stuck deleting"), got)
}

// Two causes that say the same of two objects name both once, so the
// shared words cannot be read as part of the first.
func TestRivalLeadNamesBothCausesOnceForTheSameWords(t *testing.T) {
	subject := inventory.CoreID(kube.KindDeployment, "arc", "hook")
	cause := &rootcause.CauseRecord{Rule: "finalizer-handler-stopped",
		Mode: explain.ModeHandlerStopped, Root: subject,
		Rival: &rootcause.CauseRecord{Rule: "finalizer-handler-stopped",
			Mode: explain.ModeHandlerStopped, Root: inventory.CoreID(
				kube.KindDeployment, "arc", "ctl")}}
	finding := detection.Finding{Entity: subject, Reason: "Unavailable",
		Health: detection.Failing, Summary: "Deployment is failing"}
	f := caseFacts{p: incident.Incident{Root: subject, Cause: cause,
		Members: members(finding)}, members: []detection.Finding{finding},
		lead: finding, ok: true}

	got, ok := rivalLead(f)

	require.True(t, ok)
	assert.Contains(t, got, "two possible causes: it or ctl is not "+
		"running, so it removes no finalizers")
	assert.Equal(t, "a or b", rivalChoice("a", "b", "", ""))
}

func TestOngoingTitleSpellsCustomKindsAsDeclared(t *testing.T) {
	kind := inventory.Kind("autoscalinglistener")
	root := inventory.EntityID{Kind: kind, Namespace: "arc",
		Name: "test-listener"}
	finding := detection.Finding{Entity: root, Reason: "StuckDeleting",
		Mode: detection.ModeStuckDeleting, Health: detection.Degraded,
		Severity: detection.Info, Since: ongoingNow.AddDate(0, -11, 0)}
	p := incident.Incident{ID: "inc-1", Root: root, Tier: incident.Digest,
		State: incident.Open, Opened: finding.Since,
		Members: members(finding)}

	plain := Writer{}.OngoingTitle(p, nil, ongoingNow)
	named := Writer{}.OngoingTitle(p, map[inventory.Kind]string{
		kind: "AutoscalingListener"}, ongoingNow)

	assert.Contains(t, plain, "test-listener")
	assert.Contains(t, named, "AutoscalingListener test-listener", named)
}

// The headline counts new and ongoing problems apart, and each has its
// own section, so what a section shows adds up to its count.
func TestDigestSectionsMatchTheHeadlineCounts(t *testing.T) {
	ongoing := Ongoing{Root: inventory.CoreID(kube.KindService,
		"kube-system", "x"), Title: "Service x in kube-system is still " +
		"failing, for 2 days now."}
	msg := Writer{}.DigestWith([]incident.Decision{
		leftoverDecision(0, detection.Warning)}, nil, nil,
		DigestExtras{Ongoing: []Ongoing{ongoing}}, extrasNow)

	text := msg.Render(notification.SlackDialect())
	news, rest, ok := strings.Cut(text, "*Ongoing*")
	require.True(t, ok, text)
	assert.Contains(t, news, "1 problem · 1 ongoing")
	assert.Contains(t, news, "*Problems*\n• Secret *tls*")
	assert.Equal(t, "\n• Service x in *kube-system* is still failing, "+
		"for 2 days now", rest)
}
