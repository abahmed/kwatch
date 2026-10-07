package compose

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

// Ongoing is a digest-tier problem an earlier digest listed that is still
// open. The digest says so again, with how often it was seen and since
// when, so a problem that never goes away is not forgotten.
type Ongoing struct {
	Root inventory.EntityID
	// Reason is what keeps failing, as the finding names it.
	Reason string
	// Count is how many times it was seen since Since; zero when kwatch
	// does not count it.
	Count int
	Since time.Time
}

// WakeLine is the summary of a cluster wake-up that ended: the workloads
// that started from zero replicas and the startup failures they had.
type WakeLine struct {
	Started int
	// From and To are the first and the latest start.
	From, To time.Time
	// Blips counts the workloads that had startup failures, and Failing
	// those that still do.
	Blips, Failing int
}

// DigestExtras is what a digest says beyond the incidents that opened
// and resolved in its window.
type DigestExtras struct {
	Ongoing []Ongoing
	// Wake is the summary of a wake-up; nil when none ended.
	Wake *WakeLine
}

// Empty reports nothing to add.
func (x DigestExtras) Empty() bool {
	return len(x.Ongoing) == 0 && x.Wake == nil
}

// Text words the wake-up on one line: "Cluster waking up: 42 workloads
// started between 06:51 and 07:03; 5 had brief startup failures, all
// recovered."
func (l WakeLine) Text() string {
	text := "Cluster waking up: " + countFact(l.Started, "workload") +
		" started " + startedWhen(l.From, l.To)
	if l.Blips == 0 {
		return text + "."
	}
	text += "; " + strconv.Itoa(l.Blips) + " had brief startup failures, "
	switch {
	case l.Failing == 0:
		return text + "all recovered."
	case l.Failing == l.Blips:
		return text + "none recovered yet."
	}
	return text + strconv.Itoa(l.Blips-l.Failing) + " recovered, " +
		strconv.Itoa(l.Failing) + " still failing."
}

func startedWhen(from, to time.Time) string {
	if clock(from) == clock(to) {
		return "at " + clock(from)
	}
	return "between " + clock(from) + " and " + clock(to)
}

// Line is one bullet: "Service web (shop) — still failing:
// FailedDeployModel ×14 since 17:14".
func (o Ongoing) Line(now time.Time) string {
	subject := capitalKind(shortName(o.Root))
	if o.Root.Namespace != "" {
		subject += " (" + o.Root.Namespace + ")"
	}
	what := o.Reason
	if o.Count > 1 {
		what += " ×" + strconv.Itoa(o.Count)
	}
	since := clock(o.Since)
	if o.Since.UTC().YearDay() != now.UTC().YearDay() ||
		now.Sub(o.Since) >= 24*time.Hour {
		since = o.Since.UTC().Format("Jan 2 15:04")
	}
	return subject + " — still failing: " + what + " since " + since
}

// ongoingSentences are the ongoing problems for the plain note, as many
// as a digest names, then how many more there are.
func ongoingSentences(ongoing []Ongoing, now time.Time) []sentence {
	var out []sentence
	for i, o := range ongoing {
		if i == maxSummaryNamed {
			out = append(out, sentence{part: partProof, text: sentenceCase(
				numberWord(len(ongoing)-i) + " more still failing")})
			break
		}
		out = append(out, sentence{part: partProof,
			text: endSentence(o.Line(now))})
	}
	return out
}

// ongoingBullets are the same lines as bullets of the Problems list.
func ongoingBullets(ongoing []Ongoing, now time.Time) []notification.Block {
	var out []notification.Block
	for i, o := range ongoing {
		if i == maxSummaryNamed {
			out = append(out, notification.Block{Kind: notification.Bullet,
				Spans: []notification.Span{{Text: "+" +
					strconv.Itoa(len(ongoing)-i) + " more still failing"}}})
			break
		}
		bullet := []notification.Block{{Kind: notification.Bullet,
			Spans: podSpans(o.Line(now))}}
		boldNames(bullet, []string{o.Root.Name, o.Root.Namespace})
		out = append(out, bullet[0])
	}
	return out
}
