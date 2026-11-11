package incident

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
)

// Old problems. kwatch often starts watching a cluster that has been
// broken in places for months: a Job that failed two years ago, an
// issuer nobody fixed, a backup that has not run since last spring.
// None of that is news, and none of it changes by itself. Config's
// OldProblem is the horizon past which such a problem is old:
//
//   - a low finding (one that alone would only reach the digest) that
//     was already older than the horizon when kwatch first saw it opens
//     no incident; it is only counted (OldProblems);
//   - an incident that needs attention but whose every interrupting
//     failure had already lasted longer than the horizon when it opened
//     is lowered to Low, and says why.
//
// A change brings either back: a new onset is young, and a finding that
// rises above low opens as usual. A critical failure, or one that loses
// users traffic or leaves a user-facing workload without a replica, is
// never lowered.

// levelChronicPrefix starts the level code of an incident lowered for
// its age; the whole days it had lasted when it opened follow it.
const levelChronicPrefix = "chronic:"

// lowAlone reports a failure that alone would reach no one sooner than
// the digest.
func lowAlone(s detection.Finding) bool {
	return !s.Advisory &&
		(s.Severity <= detection.Info || waitsForDigest(s))
}

// agesOut reports an old low finding that opens no incident, and keeps
// count of it. A finding already in an incident stays there, and one
// that would join a live incident of its root joins it: only a problem
// kwatch never told anyone about is left out. So does one that is part
// of a group (the leftovers of one controller, objects one cause
// explains): the group is one Low incident, not N hidden ones.
func (m *Manager) agesOut(
	now time.Time, s detection.Finding, where placement,
) bool {
	key := s.Key()
	_, member := m.byMember[key]
	if m.cfg.OldProblem <= 0 || member || !lowAlone(s) ||
		s.Since.IsZero() || now.Sub(s.Since) <= m.cfg.OldProblem ||
		m.holdsLive(where.root) || m.inGroup(s, where) {
		if was, aged := m.aged[key]; aged && rose(was, s) {
			m.rising = key
		}
		delete(m.aged, key)
		return false
	}
	if m.aged == nil {
		m.aged = map[detection.Key]detection.Finding{}
	}
	m.aged[key] = s
	return true
}

// inGroup reports a finding placed with others: a leftover of a
// backend that left several objects behind, or a failure the engine
// blames on another object.
func (m *Manager) inGroup(s detection.Finding, where placement) bool {
	if !s.Leftover.IsZero() && m.leftovers[s.Leftover] >= minLeftovers {
		return true
	}
	return where.cause != nil && where.cause.Rule != selfRow &&
		where.root != s.Entity
}

// rose reports a finding that was low on its own and no longer is: it
// got worse. Its onset is still the old one, but it is news.
func rose(was, s detection.Finding) bool {
	return lowAlone(was) && !s.Advisory && !lowAlone(s)
}

// noteRose marks p when finding s, about to become its member, rose:
// from the old problems (agesOut left the key in rising) or from the
// copy p holds. The mark is saved (Marks.Rose), so p is not lowered
// for its age again after a restart either.
func (m *Manager) noteRose(p *Incident, s detection.Finding) {
	key := s.Key()
	if was, ok := p.Members[key]; m.rising == key || ok && rose(was, s) {
		p.marks.Rose = true
	}
	m.rising = detection.Key{}
}

// forgetAged drops a cleared finding from the old problems.
func (m *Manager) forgetAged(key detection.Key) {
	delete(m.aged, key)
}

// OldProblems returns the old low findings that opened no incident, by
// object then reason. Safe for concurrent use.
func (m *Manager) OldProblems() []detection.Finding {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]detection.Finding, 0, len(m.aged))
	for _, s := range m.aged {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Entity.String(), out[j].Entity.String()
		if a != b {
			return a < b
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}

// noteChronic stores how old p's failure was when p opened (chronic).
func (m *Manager) noteChronic(p *Incident) {
	p.chronic = chronicAge(p, m.cfg.OldProblem)
}

// chronicAge is the age, when p opened, of the youngest member that may
// interrupt anyone, when every such member is older than horizon. Zero
// otherwise: a new onset, an onset kwatch does not know, none, or a
// failure that was low and got worse (Marks.Rose).
func chronicAge(p *Incident, horizon time.Duration) time.Duration {
	if horizon <= 0 || p.Opened.IsZero() || p.marks.Rose {
		return 0
	}
	var youngest time.Duration
	for _, s := range p.Members {
		if s.Advisory || waitsForDigest(s) {
			continue
		}
		age := p.Opened.Sub(s.Since)
		if s.Since.IsZero() || age <= horizon {
			return 0
		}
		if youngest == 0 || age < youngest {
			youngest = age
		}
	}
	return youngest
}

// chronicLow reports an incident that needs attention only because of
// failures that are old news: worst is the worst interrupting severity.
func chronicLow(p *Incident, worst detection.Severity) bool {
	return p.chronic > 0 && worst < detection.Critical &&
		!p.trafficLost && !p.servingDown && !p.criticalRoot()
}

// chronicFalls reports an announced notification that only its age
// lowers: it falls to the digest without a message, like an idle
// webhook, since nothing about the failure changed.
func chronicFalls(p *Incident) bool {
	worst, _ := interrupting(p)
	return !quietMembers(p) && chronicLow(p, worst)
}

// chronicLevel is the level code of an incident lowered for its age.
func chronicLevel(p *Incident) string {
	return levelChronicPrefix +
		strconv.Itoa(int(p.chronic/(24*time.Hour)))
}

// chronicText is the plain words of a chronic level code; ok is false
// for any other code.
func chronicText(code string) (string, bool) {
	days, ok := strings.CutPrefix(code, levelChronicPrefix)
	if !ok {
		return "", false
	}
	n, err := strconv.Atoi(days)
	if err != nil {
		return "", false
	}
	return "Lowered to Low: already failing for " + spokenDays(n) +
		" when kwatch first saw it, and nothing has changed " +
		"since. kwatch lists it in the daily summary and sends no " +
		"alert; a new failure makes it Needs attention again.", true
}

// spokenDays says a number of days the way a person would: "45 days",
// "about 5 months", "about 2 years".
func spokenDays(days int) string {
	const month, year = 30.44, 365.25
	unit := func(n int, word string) string {
		if n == 1 {
			return "about a " + word
		}
		return "about " + strconv.Itoa(n) + " " + word + "s"
	}
	switch {
	case days < 60:
		return strconv.Itoa(days) + " days"
	case float64(days) < year:
		return unit(int(float64(days)/month+0.5), "month")
	}
	return unit(int(float64(days)/year+0.5), "year")
}
