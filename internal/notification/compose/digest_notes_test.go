package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

var extrasNow = time.Date(2026, 10, 6, 21, 0, 0, 0, time.UTC)

func lbOngoing(count int) Ongoing {
	return Ongoing{
		Root: inventory.CoreID(kube.KindService, "kube-system",
			"internal-ingress-nginx-controller-internal"),
		Reason: "FailedDeployModel", Count: count,
		Since: time.Date(2026, 10, 6, 17, 14, 0, 0, time.UTC),
	}
}

func TestDigestListsAnOngoingProblemWithCountAndAge(t *testing.T) {
	msg := Writer{}.DigestWith(nil, nil, nil,
		DigestExtras{Ongoing: []Ongoing{lbOngoing(14)}}, extrasNow)

	assert.Equal(t, "🟡 *kwatch digest* — 1 ongoing\n\n*Problems*\n• "+
		"Service *internal-ingress-nginx-controller-internal* "+
		"(*kube-system*) — still failing: FailedDeployModel ×14 since 17:14",
		msg.Render(notification.SlackDialect()))
	assert.Contains(t, msg.Note, "one ongoing problem to report")
}

func TestOngoingProblemsShareTheDigestCap(t *testing.T) {
	var ongoing []Ongoing
	for range maxSummaryNamed + 3 {
		ongoing = append(ongoing, lbOngoing(2))
	}

	msg := Writer{}.DigestWith(nil, nil, nil,
		DigestExtras{Ongoing: ongoing}, extrasNow)

	assert.Contains(t, msg.Plain(), "- +3 more still failing")
}

func TestOngoingProblemOfAnEarlierDayNamesTheDay(t *testing.T) {
	o := lbOngoing(1)
	o.Since = extrasNow.Add(-50 * time.Hour)

	assert.Contains(t, o.Line(extrasNow), "FailedDeployModel since Oct 4")
	assert.NotContains(t, o.Line(extrasNow), "×")
}

func TestWakeLineSaysWhatRecovered(t *testing.T) {
	from := time.Date(2026, 10, 6, 6, 51, 0, 0, time.UTC)
	to := from.Add(12 * time.Minute)
	for _, c := range []struct {
		line WakeLine
		want string
	}{
		{WakeLine{Started: 42, From: from, To: to, Blips: 5},
			"Cluster waking up: 42 workloads started between 06:51 and " +
				"07:03; 5 had brief startup failures, all recovered."},
		{WakeLine{Started: 7, From: from, To: from, Blips: 2, Failing: 1},
			"Cluster waking up: 7 workloads started at 06:51; 2 had " +
				"brief startup failures, 1 recovered, 1 still failing."},
		{WakeLine{Started: 7, From: from, To: to, Blips: 2, Failing: 2},
			"Cluster waking up: 7 workloads started between 06:51 and " +
				"07:03; 2 had brief startup failures, none recovered yet."},
		{WakeLine{Started: 6, From: from, To: to},
			"Cluster waking up: 6 workloads started between 06:51 and " +
				"07:03."},
	} {
		assert.Equal(t, c.want, c.line.Text())
	}
}

func TestDigestCarriesTheWakeLineAlone(t *testing.T) {
	from := time.Date(2026, 10, 6, 6, 51, 0, 0, time.UTC)
	wake := &WakeLine{Started: 42, From: from, To: from.Add(12 * time.Minute),
		Blips: 5}

	msg := Writer{}.DigestWith(nil, nil, nil, DigestExtras{Wake: wake},
		extrasNow)

	assert.Contains(t, msg.Render(notification.SlackDialect()),
		"Cluster waking up: 42 workloads started between 06:51 and 07:03")
	assert.Contains(t, msg.Note, "has a cluster wake-up to report")
}

func TestDigestWithoutExtrasIsTheDigest(t *testing.T) {
	d := []incident.Decision{podDecision("a", incident.Digest)}
	assert.Equal(t, Writer{}.Digest(d, nil, nil, extrasNow),
		Writer{}.DigestWith(d, nil, nil, DigestExtras{}, extrasNow))
}
