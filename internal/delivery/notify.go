package delivery

import (
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/metrics"
)

// Notify queues a plain message for every provider.
//
// It used to send inline, on the caller's goroutine, with per-provider
// retries and no dead-letter record. The startup banner is sent from the
// goroutine that then starts the informers, so one unreachable provider held
// up monitoring behind three backoffs; and a message that failed everywhere
// vanished without appearing in /deadletters. Queuing puts messages on the
// same paced, digested, dead-lettered path as incidents.
func (a *Manager) Notify(msg string) {
	klog.InfoS("sending message", "messageLength", len(msg))
	a.enqueue(deliverJob{kind: jobMessage, msg: msg})
}

// NotifyEvent queues a legacy event for every provider.
func (a *Manager) NotifyEvent(ev event.Event) {
	// Kubernetes event text is an analysis input, never provider content.
	// Legacy callers receive the same boundary as incident delivery.
	ev.Events = ""
	ev.IncludeEvents = false
	klog.InfoS(
		"sending event",
		"resource", ev.Resource,
		"namespace", ev.Namespace,
		"name", ev.PodName,
		"reason", ev.Reason,
		"action", ev.Action,
	)
	a.enqueue(deliverJob{kind: jobEvent, ev: &ev})
}

// enqueue retains jobs until workers exist. It never performs provider I/O on
// the caller's goroutine before Start.
func (a *Manager) enqueue(job deliverJob) {
	a.mu.Lock()
	if a.stopped {
		if a.reconfiguring && len(a.pending) < channelCap {
			a.pending = append(a.pending, job)
			a.mu.Unlock()
			return
		}
		a.mu.Unlock()
		recordStoppedDrop()
		return
	}
	if a.started {
		a.fanOut(job)
		a.mu.Unlock()
		return
	}
	if len(a.pending) >= channelCap {
		generation := a.currentGenerationLocked()
		if generation != nil && len(generation.order) > 0 {
			entry := generation.entries[generation.order[0]]
			a.recordDeadLetter(
				&entry, job, fmt.Errorf("pending delivery queue saturated"),
			)
		}
		a.mu.Unlock()
		return
	}
	a.pending = append(a.pending, job)
	a.mu.Unlock()
}

// EventDeliveryProvider is a marker interface for providers whose real
// delivery is implemented in SendEvent (not SendMessage). PagerDuty,
// Opsgenie, Zenduty, and Email all stub SendMessage to return nil — the
// routing layer must call SendEvent instead for these providers.

type EventDeliveryProvider interface {
	Provider
	UsesEventDelivery()
}

func recordStoppedDrop() {
	metrics.DefaultRegistry().NotificationsDropped.Add(1)
	klog.V(2).InfoS("notification dropped; delivery is not accepting jobs")
}
