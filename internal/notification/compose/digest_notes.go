package compose

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

// Ongoing is a digest-tier problem an earlier digest listed that is still
// open. The digest says so again, in the words the incident's own
// reminder uses, so a problem that never goes away is not forgotten.
type Ongoing struct {
	Root inventory.EntityID
	// Title is the one-line description of the problem (see
	// Writer.OngoingTitle); empty when the writer had none.
	Title string
	// Since is when the problem began (see StartedAt).
	Since time.Time
	// Leftover is the reason that makes it a leftover that groups with
	// the others of its reason (see LeftoverReason); empty otherwise.
	Leftover string
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
	// Ongoing are the ongoing problems listed in full: new to the day
	// or changed since they were last listed.
	Ongoing []Ongoing
	// Unchanged are ongoing problems this day's digests already listed
	// in full and that did not change: one summary line names them.
	Unchanged []Ongoing
	// Wake is the summary of a wake-up; nil when none ended.
	Wake *WakeLine
}

// Empty reports nothing to add.
func (x DigestExtras) Empty() bool {
	return len(x.Ongoing) == 0 && len(x.Unchanged) == 0 && x.Wake == nil
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

// Line is one bullet: "Service web (shop) is still failing, for two days
// now." Without a title it says only the subject and when it began:
// "Service web (shop) — still failing since 17:14". A reason code is never
// part of it.
func (o Ongoing) Line(now time.Time) string {
	if o.Title != "" {
		return o.Title
	}
	subject := capitalKind(shortName(o.Root))
	if o.Root.Namespace != "" {
		subject += " (" + o.Root.Namespace + ")"
	}
	since := clock(o.Since)
	if o.Since.UTC().YearDay() != now.UTC().YearDay() ||
		now.Sub(o.Since) >= 24*time.Hour {
		since = o.Since.UTC().Format("Jan 2 15:04")
	}
	return subject + " — still failing since " + since
}

// maxUnchangedNamed is how many unchanged problems the summary line
// names before "+K".
const maxUnchangedNamed = 3

// unchangedLine is the one line for the problems that did not change:
// "2 more still failing, unchanged (web, db +1)". Empty when none.
func unchangedLine(unchanged []Ongoing) string {
	if len(unchanged) == 0 {
		return ""
	}
	var names []string
	for _, o := range unchanged[:min(len(unchanged), maxUnchangedNamed)] {
		names = append(names, o.Root.Name)
	}
	list := strings.Join(names, ", ")
	if extra := len(unchanged) - len(names); extra > 0 {
		list += " +" + strconv.Itoa(extra)
	}
	return strconv.Itoa(len(unchanged)) + " more still failing, " +
		"unchanged (" + list + ")"
}

// ongoingSentences are the ongoing problems for the plain note, as many
// as a digest names, then how many more there are, then the line for
// the unchanged ones.
func ongoingSentences(extras DigestExtras, now time.Time) []sentence {
	ongoing := extras.Ongoing
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
	if line := unchangedLine(extras.Unchanged); line != "" {
		out = append(out, sentence{part: partProof,
			text: endSentence(line)})
	}
	return out
}

// ongoingBullets are the same lines as bullets of the Problems list.
func ongoingBullets(extras DigestExtras, now time.Time) []notification.Block {
	ongoing := extras.Ongoing
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
	if line := unchangedLine(extras.Unchanged); line != "" {
		out = append(out, notification.Block{Kind: notification.Bullet,
			Spans: []notification.Span{{Text: line}}})
	}
	return out
}
