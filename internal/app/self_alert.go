package app

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/notification"
)

// Self alerts: one quiet message when a part of kwatch itself stays
// degraded, updated in place when what is wrong changes, and one "back
// to normal" when it recovers. kwatch cannot say it is down; that is
// the heartbeat's job.
const (
	// selfAlertHold is how long a part must stay degraded before it is
	// told, and how long a session runs before an open message may be
	// closed: a short restart or a blip says nothing.
	selfAlertHold = 10 * time.Minute
	// selfAlertEvery is how often the parts are checked.
	selfAlertEvery = 30 * time.Second
	// selfAlertsPerDay caps the messages that open, per UTC day.
	selfAlertsPerDay = 6
	// stateSelfAlerts holds the open self alerts and the day's count.
	stateSelfAlerts = "self.alerts"
)

// selfCondition is one degraded part of kwatch: its health component
// (or a probe of its own) and the reason.
type selfCondition struct {
	Part, Reason string
}

// selfOpen is a self alert that went out and is not closed yet. Its
// JSON shape is persisted.
type selfOpen struct {
	Reason   string    `json:"reason"`
	Since    time.Time `json:"since"`
	Revision int       `json:"revision"`
	// Pending is a newer reason not told yet: updates are spaced by
	// selfAlertHold, and the next update or the close carries it.
	Pending string `json:"pending,omitempty"`
	// Updated is when the last update went out.
	Updated time.Time `json:"updated,omitzero"`
}

// selfAlertState is what survives a restart, so a night off does not
// repeat a message or lose its "back to normal". Its JSON shape is
// persisted.
type selfAlertState struct {
	Open map[string]selfOpen `json:"open,omitempty"`
	Day  string              `json:"day,omitempty"`
	Sent int                 `json:"sent,omitempty"`
	// Granted holds the parts seen working at least once: an optional
	// permission refused from the start is a choice, not a fault.
	Granted map[string]bool `json:"granted,omitempty"`
}

// selfAlertStore keeps selfAlertState.
type selfAlertStore interface {
	loadSelfAlerts() (selfAlertState, error)
	saveSelfAlerts(selfAlertState) error
}

// selfAlertSink queues a message for delivery.
type selfAlertSink interface {
	NotifyIncident(notification.Message)
}

// selfAlerter turns degraded parts into self alerts.
type selfAlerter struct {
	conditions func(now time.Time) []selfCondition
	// proven lists the parts seen working now; nil means none.
	proven  func() []string
	sink    selfAlertSink
	store   selfAlertStore
	now     func() time.Time
	cluster string
	// updates allows edits of an open message (threads, Pro); without
	// them only the opening and the close are sent.
	updates bool
	started time.Time
	// seen is when each degraded part was first seen degraded in a row.
	seen    map[string]time.Time
	state   selfAlertState
	loaded  bool
	capSaid string
}

// run checks the parts every selfAlertEvery until ctx ends.
func (a *selfAlerter) run(ctx context.Context) error {
	ticker := time.NewTicker(selfAlertEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			a.step(a.now())
		}
	}
}

// step is one check at now: open, update and close self alerts.
func (a *selfAlerter) step(now time.Time) {
	if !a.loadState() {
		return
	}
	if a.started.IsZero() {
		a.started = now
	}
	current := map[string]selfCondition{}
	for _, c := range a.conditions(now) {
		current[c.Part] = c
	}
	changed := a.recordGrants()
	a.dropNeverGranted(current)
	a.trackSeen(current, now)
	for part, c := range current {
		changed = a.stepDegraded(part, c, now) || changed
	}
	for part, open := range a.state.Open {
		if _, still := current[part]; !still {
			changed = a.close(part, open, now) || changed
		}
	}
	if changed {
		a.saveState()
	}
}

// trackSeen remembers when each part became degraded and forgets the
// ones that are not degraded now. It changes no persisted state.
func (a *selfAlerter) trackSeen(
	current map[string]selfCondition, now time.Time,
) {
	for part := range a.seen {
		if _, ok := current[part]; !ok {
			delete(a.seen, part)
		}
	}
	for part := range current {
		if _, ok := a.seen[part]; !ok {
			a.seen[part] = now
		}
	}
}

// stepDegraded opens the alert of a part degraded for selfAlertHold,
// or updates the open one when its reason changed. It reports whether
// the persisted state changed.
func (a *selfAlerter) stepDegraded(
	part string, c selfCondition, now time.Time,
) bool {
	open, isOpen := a.state.Open[part]
	if isOpen {
		return a.update(part, open, c.Reason, now)
	}
	since := a.seen[part]
	if now.Sub(since) < selfAlertHold || !a.withinCap(now) {
		return false
	}
	open = selfOpen{Reason: c.Reason, Since: since, Revision: 1}
	a.state.Open[part] = open
	a.state.Sent++
	a.sink.NotifyIncident(selfMessage(a.cluster, part, open))
	klog.InfoS("kwatch is degraded; told the default channel",
		"component", "self-alerts", "part", part, "reason", c.Reason)
	return true
}

// update records reason on the open alert of part. Edits of the
// message are spaced by selfAlertHold, so a flapping reason cannot send
// one per check; the newest reason waits in Pending. It reports whether
// the persisted state changed.
func (a *selfAlerter) update(
	part string, open selfOpen, reason string, now time.Time,
) bool {
	pending := reason
	if reason == open.Reason {
		pending = ""
	}
	if !a.updates || pending == "" {
		changed := open.Reason != reason || open.Pending != ""
		open.Reason, open.Pending = reason, ""
		a.state.Open[part] = open
		return changed
	}
	if !open.Updated.IsZero() && now.Sub(open.Updated) < selfAlertHold {
		changed := open.Pending != pending
		open.Pending = pending
		a.state.Open[part] = open
		return changed
	}
	open.Reason, open.Pending, open.Updated = pending, "", now
	open.Revision++
	a.sink.NotifyIncident(selfMessage(a.cluster, part, open))
	a.state.Open[part] = open
	return true
}

// close sends "back to normal" for a part no longer degraded, once
// this session has run long enough for every part to report.
func (a *selfAlerter) close(part string, open selfOpen, now time.Time) bool {
	if now.Sub(a.started) < selfAlertHold {
		return false
	}
	if open.Pending != "" {
		open.Reason, open.Pending = open.Pending, ""
	}
	open.Revision++
	a.sink.NotifyIncident(selfResolved(a.cluster, part, open, now))
	delete(a.state.Open, part)
	return true
}

// withinCap reports whether another self alert may open today (UTC),
// logging once a day when the cap holds one back.
func (a *selfAlerter) withinCap(now time.Time) bool {
	day := now.UTC().Format(time.DateOnly)
	if a.state.Day != day {
		a.state.Day, a.state.Sent = day, 0
	}
	if a.state.Sent < selfAlertsPerDay {
		return true
	}
	if a.capSaid != day {
		a.capSaid = day
		klog.InfoS("self alert held back: daily cap reached",
			"component", "self-alerts", "cap", selfAlertsPerDay)
	}
	return false
}

// loadState reads the saved state once; a read failure is logged and
// the check skipped, so nothing is sent twice.
func (a *selfAlerter) loadState() bool {
	if a.loaded {
		return true
	}
	if a.seen == nil {
		a.seen = map[string]time.Time{}
	}
	state, err := a.store.loadSelfAlerts()
	if err != nil {
		klog.ErrorS(err, "read self alerts", "component", "self-alerts")
		return false
	}
	if state.Open == nil {
		state.Open = map[string]selfOpen{}
	}
	a.state, a.loaded = state, true
	return true
}

// saveState writes the state; a failure is logged and retried with the
// next change.
func (a *selfAlerter) saveState() {
	if err := a.store.saveSelfAlerts(a.state); err != nil {
		klog.ErrorS(err, "save self alerts", "component", "self-alerts")
	}
}

// loadSelfAlerts implements selfAlertStore.
func (d diskState) loadSelfAlerts() (selfAlertState, error) {
	var state selfAlertState
	_, err := d.get(stateSelfAlerts, &state)
	return state, err
}

// saveSelfAlerts implements selfAlertStore.
func (d diskState) saveSelfAlerts(state selfAlertState) error {
	return d.put(stateSelfAlerts, state)
}
