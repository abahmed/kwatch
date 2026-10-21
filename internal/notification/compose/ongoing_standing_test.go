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
	"github.com/abahmed/kwatch/internal/notification"
)

func standingOngoing(name string) Ongoing {
	return Ongoing{Standing: true,
		Root: inventory.CoreID(kube.KindQuota, "batch", name)}
}

// A condition that fails nothing is "open", never "failing", in every
// line a digest has for an ongoing problem.
func TestOngoingLineOfAStandingConditionDoesNotSayFailing(t *testing.T) {
	o := standingOngoing("pod-cap")
	o.Since = extrasNow.Add(-time.Hour)

	assert.Contains(t, o.Line(extrasNow), "still open since")
	assert.NotContains(t, o.Line(extrasNow), "failing")
}

func TestUnchangedLineOfStandingConditionsSaysStillOpen(t *testing.T) {
	failing := Ongoing{Root: inventory.CoreID(kube.KindDeployment, "shop",
		"web")}

	assert.Equal(t, "one more still open, unchanged (pod-cap)",
		unchangedLine([]Ongoing{standingOngoing("pod-cap")}))
	assert.Equal(t, "two more still open, unchanged (web, pod-cap)",
		unchangedLine([]Ongoing{failing, standingOngoing("pod-cap")}),
		"a list with a condition in it is not all failures")
	assert.Equal(t, "one more still failing, unchanged (web)",
		unchangedLine([]Ongoing{failing}))
}

func TestMoreOngoingConditionsAreStillOpenNotFailing(t *testing.T) {
	var ongoing []Ongoing
	for range maxSummaryNamed + 2 {
		ongoing = append(ongoing, standingOngoing("pod-cap"))
	}

	msg := Writer{}.DigestWith(nil, nil, nil,
		DigestExtras{Ongoing: ongoing}, extrasNow)

	assert.Contains(t, msg.Plain(), "- +2 more still open")
	assert.Contains(t, msg.Note, "Two more still open")
	assert.NotContains(t, msg.Plain(), "more still failing")
	assert.NotContains(t, msg.Note, "more still failing")
	var bullets []string
	for _, b := range msg.Doc {
		if b.Kind == notification.Bullet {
			bullets = append(bullets, notification.Text(
				notification.Message{Doc: []notification.Block{b}}))
		}
	}
	assert.NotEmpty(t, bullets)
}

// A condition that degrades nothing, or a risk the incident only holds
// as advice, is standing; a failure is not.
func TestIsStandingTellsConditionsFromFailures(t *testing.T) {
	quota := inventory.CoreID(kube.KindQuota, "batch", "pod-cap")
	web := inventory.CoreID(kube.KindDeployment, "shop", "web")
	with := func(root inventory.EntityID, m ...detection.Finding,
	) incident.Incident {
		return incident.Incident{ID: "i", Root: root, Members: members(m...)}
	}
	cond := detection.Finding{Entity: quota, Reason: "ResourceQuotaExhausted",
		Health: detection.Degraded}
	fail := detection.Finding{Entity: web, Reason: "CrashLoopBackOff",
		Health: detection.Failing}
	risk := detection.Finding{Entity: web, Advisory: true,
		Reason: reasons.RiskSingleReplica}

	assert.True(t, IsStanding(with(quota, cond)))
	assert.True(t, IsStanding(with(web, risk)))
	assert.False(t, IsStanding(with(web, fail)))
	assert.False(t, IsStanding(with(web, fail, risk)))
	assert.False(t, IsStanding(with(web)), "nothing known: not a condition")
}

func TestReminderOfAnAdvisoryOnlyIncidentDoesNotSayFailing(t *testing.T) {
	web := inventory.CoreID(kube.KindDeployment, "shop", "web")
	p := advisoryIncident(web, riskFinding(web))
	p.Tier = incident.Digest

	title := Writer{}.OngoingTitle(p, advisoryNow.Add(48*time.Hour))

	assert.NotContains(t, title, "still failing")
}
