package delivery

import (
	"context"
	"text/template"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/notification"
)

// NotifyIncident queues one incident message for every provider. It never
// performs provider I/O on the caller's goroutine.
func (m *Manager) NotifyIncident(msg notification.Message) {
	if msg.Carrier != "" {
		// Recorded for the audit log; the digest or summary carries it.
		return
	}
	klog.V(2).InfoS("queue incident", "component", "delivery",
		"conversation", msg.Key, "revision", msg.Revision)
	incident := msg
	m.enqueue(deliverJob{kind: jobIncident, incident: &incident})
}

// dispatchIncident hands the message to the provider's own renderer. The
// manager only prepares the text every renderer shares: a user template,
// the fallback prefix and the provider's payload limit.
func (m *Manager) dispatchIncident(
	ctx context.Context,
	entry *providerEntry,
	job deliverJob,
	opts deliverOpts,
) error {
	p := entry.provider
	templates := entry.templates
	if len(templates) == 0 {
		templates = m.globalTemplates()
	}
	msg := m.makeUpForLostOpen(entry, *job.incident)
	msg = prepareIncident(msg, templates, opts.fallbackFrom, entry.maxBytes)
	requestCtx := m.requestContext(ctx)
	return sendWithRetry(ctx, func() error {
		return p.SendIncident(requestCtx, msg)
	}, opts.retry, p.Name())
}

// prepareIncident applies the user template and the fallback notice to
// the narrative, then cuts Note and Short to the provider's byte limit.
// The notice goes last so the status marker stays the first character.
// The cut is deterministic, so a retry sends the same payload.
func prepareIncident(
	m notification.Message,
	templates map[string]*template.Template,
	fallbackFrom string,
	maxBytes int,
) notification.Message {
	m.Note = incidentNote(m, templates)
	if fallbackFrom != "" {
		m.Note += "\n(Sent through the fallback because " + fallbackFrom +
			" failed.)"
	}
	if maxBytes > 0 {
		m.Note = notification.Truncate(m.Note, maxBytes)
		m.Short = notification.Truncate(m.ShortText(), maxBytes)
	}
	return m
}
