package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

var extrasNow = time.Date(2026, 10, 6, 21, 0, 0, 0, time.UTC)

func lbOngoing() Ongoing {
	return Ongoing{
		Root: inventory.CoreID(kube.KindService, "kube-system",
			"internal-ingress-nginx-controller-internal"),
		Since: time.Date(2026, 10, 6, 17, 14, 0, 0, time.UTC),
	}
}

func TestDigestListsAnOngoingProblemWithItsAge(t *testing.T) {
	msg := Writer{}.DigestWith(nil, nil, nil,
		DigestExtras{Ongoing: []Ongoing{lbOngoing()}}, extrasNow)

	assert.Equal(t, "🟡 *kwatch summary* — 1 ongoing\n\n*Ongoing*\n• "+
		"Service *internal-ingress-nginx-controller-internal* "+
		"(*kube-system*) — still failing since 17:14 UTC",
		msg.Render(notification.SlackDialect()))
	assert.Contains(t, msg.Note, "one problem still open")
}

func TestOngoingProblemsShareTheDigestCap(t *testing.T) {
	var ongoing []Ongoing
	for range maxSummaryNamed + 3 {
		ongoing = append(ongoing, lbOngoing())
	}

	msg := Writer{}.DigestWith(nil, nil, nil,
		DigestExtras{Ongoing: ongoing}, extrasNow)

	assert.Contains(t, msg.Plain(), "- +3 more still failing")
}

func TestOngoingProblemOfAnEarlierDayNamesTheDay(t *testing.T) {
	o := lbOngoing()
	o.Since = extrasNow.Add(-50 * time.Hour)

	assert.Contains(t, o.Line(extrasNow), "still failing since Oct 4")
}

// A titled problem reads exactly as the title: the incident's own words.
func TestOngoingProblemUsesItsTitle(t *testing.T) {
	o := lbOngoing()
	o.Title = "Service x (kube-system) is still failing, for 2 days now."

	assert.Equal(t, o.Title, o.Line(extrasNow))
}

func TestWakeLineSaysWhatRecovered(t *testing.T) {
	from := time.Date(2026, 10, 6, 6, 51, 0, 0, time.UTC)
	to := from.Add(12 * time.Minute)
	for _, c := range []struct {
		line WakeLine
		want string
	}{
		{WakeLine{Started: 42, From: from, To: to, Blips: 5},
			"Cluster waking up: 42 workloads started between 06:51 UTC and " +
				"07:03 UTC; 5 had brief startup failures, all recovered."},
		{WakeLine{Started: 7, From: from, To: from, Blips: 2, Failing: 1},
			"Cluster waking up: 7 workloads started at 06:51 UTC; 2 had " +
				"brief startup failures, 1 recovered, 1 still failing."},
		{WakeLine{Started: 7, From: from, To: to, Blips: 2, Failing: 2},
			"Cluster waking up: 7 workloads started between 06:51 UTC and " +
				"07:03 UTC; 2 had brief startup failures, none recovered yet."},
		{WakeLine{Started: 6, From: from, To: to},
			"Cluster waking up: 6 workloads started between 06:51 UTC and " +
				"07:03 UTC."},
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
		"Cluster waking up: 42 workloads started between 06:51 UTC and 07:03 UTC")
	assert.Contains(t, msg.Note, "kwatch summary: a cluster wake-up.")
}

func TestDigestWithoutExtrasIsTheDigest(t *testing.T) {
	d := []incident.Decision{podDecision("a", incident.Digest)}
	assert.Equal(t, Writer{}.Digest(d, nil, nil, extrasNow),
		Writer{}.DigestWith(d, nil, nil, DigestExtras{}, extrasNow))
}

// Unchanged problems are one summary line, named up to a cap, however
// many there are; the digest still counts them as ongoing.
func TestUnchangedOngoingProblemsAreOneLine(t *testing.T) {
	var unchanged []Ongoing
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		o := lbOngoing()
		o.Root.Name = name
		unchanged = append(unchanged, o)
	}

	msg := Writer{}.DigestWith(nil, nil, nil,
		DigestExtras{Unchanged: unchanged}, extrasNow)

	assert.Contains(t, msg.Plain(),
		"- Still failing and unchanged (5): a, b, c +2")
	assert.Contains(t, msg.Note, "five problems still open")
	assert.Contains(t, msg.Note,
		"Still failing and unchanged (5): a, b, c +2.")
	assert.False(t, DigestExtras{Unchanged: unchanged}.Empty())
}

// A changed problem is listed in full beside the line of the rest.
func TestChangedAndUnchangedOngoingProblemsShareADigest(t *testing.T) {
	changed := lbOngoing()
	changed.Title = "Service x (kube-system) is still failing, for 2 days now."
	other := lbOngoing()
	other.Root.Name = "db"

	msg := Writer{}.DigestWith(nil, nil, nil, DigestExtras{
		Ongoing: []Ongoing{changed}, Unchanged: []Ongoing{other}},
		extrasNow)

	assert.Contains(t, msg.Plain(), "- "+strings.TrimSuffix(changed.Title,
		".")+"\n", "a digest bullet has no closing period")
	assert.Contains(t, msg.Plain(), "- Still failing and unchanged: db")
}
