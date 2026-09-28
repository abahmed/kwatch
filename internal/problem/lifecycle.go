package problem

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

// Tick advances every problem's lifecycle and returns the decisions that
// warrant a message, plus the delay until the next deadline (zero when
// none is pending).
func (m *Manager) Tick(now time.Time) ([]Decision, time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var decisions []Decision
	var next time.Duration
	wake := func(at time.Time) {
		if delay := at.Sub(now); delay > 0 && (next == 0 || delay < next) {
			next = delay
		}
	}
	for _, id := range m.sortedIDs() {
		p := m.problems[id]
		if d, ok := m.advance(p, now, wake); ok {
			decisions = append(decisions, d)
		}
		if p.State == Resolved && now.Sub(p.Resolved) > m.cfg.Remember {
			delete(m.problems, id)
		}
	}
	return decisions, next
}

func (m *Manager) advance(
	p *Problem, now time.Time, wake func(time.Time),
) (Decision, bool) {
	switch p.State {
	case Settling:
		return m.settle(p, now, wake)
	case Open:
		return m.open(p, now)
	case Recovering:
		return m.recovering(p, now, wake)
	case Flapping:
		return m.flapping(p, now, wake)
	}
	return Decision{}, false
}

func (m *Manager) settle(
	p *Problem, now time.Time, wake func(time.Time),
) (Decision, bool) {
	if len(p.Members) == 0 && now.Before(m.restoreGrace) {
		return Decision{}, false
	}
	if len(p.Members) == 0 {
		// Recovered before anyone was told: stay silent.
		p.State, p.Resolved = Resolved, now
		return Decision{}, false
	}
	settle := m.cfg.Settle
	if p.Tier == Page {
		settle = m.cfg.PageSettle
	}
	if due := p.Opened.Add(settle); now.Before(due) {
		wake(due)
		return Decision{}, false
	}
	if p.Tier == Silent {
		return Decision{}, false
	}
	p.State, p.Announced = Open, now
	return m.decide(p, Announce, "settled"), true
}

func (m *Manager) open(p *Problem, now time.Time) (Decision, bool) {
	if len(p.Members) == 0 && now.Before(m.restoreGrace) {
		return Decision{}, false
	}
	if len(p.Members) == 0 {
		p.State, p.RecoveringSince = Recovering, now
		return Decision{}, false
	}
	if digest(p) == p.Digest {
		return Decision{}, false
	}
	return m.decide(p, Update, "material change"), true
}

func (m *Manager) recovering(
	p *Problem, now time.Time, wake func(time.Time),
) (Decision, bool) {
	if len(p.Members) > 0 {
		// Failed again inside the hold: same problem, no new message.
		p.Cycles = recent(append(p.Cycles, now), now, m.cfg.FlapWindow)
		if len(p.Cycles) >= m.cfg.FlapCycles {
			p.State = Flapping
			p.note(now, "flapping: "+strconv.Itoa(len(p.Cycles))+
				" recoveries in "+m.cfg.FlapWindow.String())
			return m.decide(p, Update, "flapping"), true
		}
		p.State = Open
		return Decision{}, false
	}
	hold := m.cfg.hold(len(recent(p.Cycles, now, m.cfg.FlapWindow)))
	if due := p.RecoveringSince.Add(hold); now.Before(due) {
		wake(due)
		return Decision{}, false
	}
	return m.resolve(p, now, "healthy for "+hold.String()), true
}

func (m *Manager) flapping(
	p *Problem, now time.Time, wake func(time.Time),
) (Decision, bool) {
	if len(p.Members) > 0 {
		p.RecoveringSince = time.Time{}
		return Decision{}, false
	}
	if p.RecoveringSince.IsZero() {
		p.RecoveringSince = now
	}
	if due := p.RecoveringSince.Add(m.cfg.MaxHold); now.Before(due) {
		wake(due)
		return Decision{}, false
	}
	return m.resolve(p, now, "stable for "+m.cfg.MaxHold.String()), true
}

func (m *Manager) resolve(p *Problem, now time.Time, why string) Decision {
	p.State, p.Resolved = Resolved, now
	p.note(now, "resolved: "+why)
	return m.decide(p, Resolve, why)
}

func (m *Manager) decide(p *Problem, action Action, why string) Decision {
	p.Revision++
	p.Digest = digest(p)
	return Decision{Action: action, Problem: p.Snapshot(), Reason: why}
}

func (m *Manager) sortedIDs() []string {
	ids := make([]string, 0, len(m.problems))
	for id := range m.problems {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// digest fingerprints what a reader would notice: tier, root, cause, the
// root's own conditions and the size of the impact. Symptoms of affected
// entities count only through the impact size, and counters and
// timestamps never trigger an update.
func digest(p *Problem) string {
	reasons := make([]string, 0, len(p.Members))
	for key, s := range p.Members {
		if key.Entity == p.Root && !s.Symptom {
			reasons = append(reasons, key.Reason)
		}
	}
	sort.Strings(reasons)
	cause := ""
	if p.Cause != nil {
		cause = p.Cause.Summary
	}
	parts := []string{
		strconv.Itoa(int(p.Tier)), p.Root.String(), cause,
		strings.Join(uniq(reasons), ","), impactBucket(impactSize(p)),
		strconv.Itoa(int(p.State)),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:8])
}

// impactSize counts affected workloads, Services and Ingresses. Pods and
// containers are excluded: replicas failing one by one are not news.
func impactSize(p *Problem) int {
	n := 0
	for _, id := range p.Impact {
		if id.Kind != kube.KindPod && id.Kind != kube.KindContainer {
			n++
		}
	}
	return n
}

// impactBucket changes only when impact crosses 1 → N or roughly doubles.
func impactBucket(n int) string {
	switch {
	case n <= 1:
		return "1"
	case n <= 3:
		return "2-3"
	case n <= 7:
		return "4-7"
	case n <= 15:
		return "8-15"
	default:
		return "16+"
	}
}

func recent(
	times []time.Time, now time.Time, window time.Duration,
) []time.Time {
	out := times[:0:0]
	for _, t := range times {
		if now.Sub(t) <= window {
			out = append(out, t)
		}
	}
	return out
}

func uniq(sorted []string) []string {
	out := sorted[:0:0]
	for i, v := range sorted {
		if i == 0 || sorted[i-1] != v {
			out = append(out, v)
		}
	}
	return out
}
