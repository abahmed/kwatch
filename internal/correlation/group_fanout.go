package correlation

import (
	"fmt"
	"hash/crc32"
	"sort"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

// fanOutMerge is the outcome of collapsing per-owner buffers into one
// namespace-level notification.
type fanOutMerge struct {
	transitions []transition
	consumed    map[string]bool
}

// mergeNamespaceFanOut collapses buffers that differ only by owner.
//
// Most reasons group by reason+namespace+owner, which is right when one
// workload is unhealthy and wrong when the whole namespace is: a node going
// away made twelve deployments unready at once and produced twelve alerts for
// one event. Mass-failure detection catches that when the resource graph links
// the failures; this catches it when the graph does not.
//
// Only fires at NamespaceFanOutThreshold distinct owners failing the same way
// inside one grouping window, which is a systemic signal rather than a
// coincidence. Caller must hold e.mu.
func (e *Engine) mergeNamespaceFanOut(
	ready []string,
	now time.Time,
) fanOutMerge {
	out := fanOutMerge{consumed: map[string]bool{}}
	threshold := e.config.NamespaceFanOutThreshold
	if threshold <= 0 {
		return out
	}

	buckets := e.fanOutBuckets(ready)
	scopes := make([]string, 0, len(buckets))
	for scope := range buckets {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)

	for _, scope := range scopes {
		keys := buckets[scope]
		entries, firstSeen := e.activeEntriesAcross(keys, now)

		// The first owner in this window was announced immediately rather than
		// buffered. It is still part of the fan-out: count it, fold it in, and
		// close its individual thread so the namespace-wide alert owns it.
		announced := e.announcedEntriesFor(scope, entries)
		if len(keys)+len(announced) < threshold {
			continue
		}
		entries = append(announced, entries...)
		if len(entries) < threshold {
			continue
		}
		for _, ge := range announced {
			if inc, ok := e.state[ge.key]; ok {
				if inc.FirstSeen.Before(firstSeen) {
					firstSeen = inc.FirstSeen
				}
				out.transitions = append(
					out.transitions,
					e.noteWiderBlastRadius(inc, now),
				)
			}
		}

		// A per-owner group announced in an earlier window now folds into the
		// namespace-wide one. Close its thread, or it stays open with no
		// resolve ever arriving.
		for _, ge := range entries {
			gk := ge.reason + "|" + ge.namespace + "|" + ge.owner
			if t, ok := e.foldAnnouncedGroup(gk, ge.reason, now); ok {
				out.transitions = append(out.transitions, t)
			}
		}

		fanKey := scope + "|*"
		pg := &pendingGroup{firstSeen: firstSeen, entries: entries}
		e.groupBuffers[fanKey] = pg
		if t, ok := e.flushOneGroup(fanKey, pg, now); ok {
			out.transitions = append(out.transitions, t)
		}
		delete(e.groupBuffers, fanKey)
		// The first owner already has a thread of its own. It is counted in
		// the group's total, but its resolve must arrive on that thread when
		// it actually recovers — not be swallowed into the group's batch
		// resolve — so it is not tracked as a group member.
		if tracker := e.groupResolveTrackers[fanKey]; tracker != nil {
			for _, ge := range announced {
				delete(tracker.members, ge.key)
			}
		}
		for _, gk := range keys {
			out.consumed[gk] = true
		}
		klog.V(2).InfoS("collapsed a namespace fan-out",
			"scope", scope, "owners", len(keys), "members", len(entries))
	}
	return out
}

// fanOutBuckets groups ready buffer keys by reason|namespace, keeping only
// keys of the default reason|namespace|owner shape.
//
// Keys scoped to a node, image, or log signature already describe a shared
// cause and must not be merged away. Several of those also have three parts
// ("r|node|<name>", "r|ns|<namespace>", "r|cp|<ns>"), so the shape is
// verified against the buffered entries rather than inferred from the
// separator count — otherwise three different nodes failing would be
// collapsed into one "namespace". Caller must hold e.mu.
func (e *Engine) fanOutBuckets(ready []string) map[string][]string {
	buckets := map[string][]string{}
	for _, gk := range ready {
		parts := strings.Split(gk, "|")
		if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
			continue
		}
		pg := e.groupBuffers[gk]
		if pg == nil || len(pg.entries) == 0 {
			continue
		}
		first := pg.entries[0]
		if first.namespace != parts[1] || first.owner != parts[2] {
			continue
		}
		scope := parts[0] + "|" + parts[1]
		buckets[scope] = append(buckets[scope], gk)
	}
	return buckets
}

// announcedEntriesFor returns synthetic group entries for owners in the scope's
// window that were announced immediately and are not already buffered, so the
// fan-out can count and absorb them. Caller must hold e.mu.
func (e *Engine) announcedEntriesFor(
	scope string,
	buffered []groupEntry,
) []groupEntry {
	w := e.fanOutWindows[scope]
	if w == nil {
		return nil
	}
	have := make(map[string]bool, len(buffered))
	for _, ge := range buffered {
		have[ge.owner] = true
	}
	owners := make([]string, 0, len(w.announced))
	for owner := range w.announced {
		if !have[owner] {
			owners = append(owners, owner)
		}
	}
	sort.Strings(owners)
	var out []groupEntry
	for _, owner := range owners {
		inc, ok := e.state[w.announced[owner]]
		if !ok || inc.State != model.StateActive {
			continue
		}
		pod := ""
		for p := range inc.Resources {
			if pod == "" || p < pod {
				pod = p
			}
		}
		out = append(out, groupEntry{
			key:             inc.Key,
			namespace:       inc.Namespace,
			owner:           owner,
			reason:          inc.Reason,
			kind:            inc.Resource,
			podName:         pod,
			nodeName:        inc.NodeName,
			image:           inc.Image,
			containerName:   inc.ContainerName,
			prevNotifiedSig: inc.NotifiedSig,
		})
	}
	return out
}

// noteWiderBlastRadius posts an update on the thread of the first owner, which
// was announced on its own before the fan-out became visible, so the reader
// knows why a namespace-wide alert is arriving. The incident stays active,
// keeps
// its thread, and resolves there when it actually recovers. It must not be
// marked resolved here: a green check for a pod that is still down is worse
// than an extra message.
// Caller must hold e.mu.
func (e *Engine) noteWiderBlastRadius(
	inc *model.Incident,
	now time.Time,
) transition {
	note := inc.Clone()
	note.LastSeen = now
	note.Hint = "this is part of a wider failure — see the namespace-wide " +
		"alert for the full list"
	return transition{note, model.ActionUpdate}
}

// foldAnnouncedGroup resolves a per-owner group that was announced before a
// namespace fan-out absorbed it, and forgets its flush and resolve state so
// the fan-out key owns those members from now on. Reports whether a resolve
// transition was produced. Caller must hold e.mu.
func (e *Engine) foldAnnouncedGroup(
	gk, reason string,
	now time.Time,
) (transition, bool) {
	fs, ok := e.groupFlushStates[gk]
	delete(e.groupFlushStates, gk)
	delete(e.groupResolveTrackers, gk)
	if !ok || !fs.notified {
		return transition{}, false
	}
	key := model.IncidentKey(groupKeyPrefix + gk)
	parts := strings.Split(gk, "|")
	ns := ""
	if len(parts) == 3 {
		ns = parts[1]
	}
	firstSeen := fs.firstSeen
	if firstSeen.IsZero() {
		firstSeen = now
	}
	return transition{&model.Incident{
		Subject: model.Subject{
			ID:        fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(key))),
			Key:       key,
			Reason:    reason,
			Namespace: ns,
			Name:      "folded into the namespace-wide alert",
		},
		Status: model.Status{
			FirstSeen: firstSeen,
			LastSeen:  now,
			State:     model.StateResolved,
		},
		Evidence: model.Evidence{
			Hint: "this group now continues as part of the namespace-wide " +
				"alert",
		},
	},

		model.ActionResolved}, true
}

// activeEntriesAcross gathers the still-active members of several buffers and
// the earliest time any of them opened. Caller must hold e.mu.
func (e *Engine) activeEntriesAcross(
	keys []string,
	now time.Time,
) ([]groupEntry, time.Time) {
	var entries []groupEntry
	firstSeen := now
	for _, gk := range keys {
		pg := e.groupBuffers[gk]
		if pg == nil {
			continue
		}
		if pg.firstSeen.Before(firstSeen) {
			firstSeen = pg.firstSeen
		}
		for _, ge := range pg.entries {
			if inc, ok := e.state[ge.key]; ok &&
				inc.State != model.StateResolved &&
				inc.State != model.StatePendingResolve {
				entries = append(entries, ge)
			}
		}
	}
	return entries, firstSeen
}
