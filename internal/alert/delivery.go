package alert

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/llm"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
)

type deliverJob struct {
	inc     *model.Incident
	action  model.IncidentAction
	insight *insight.Insight
}

type DeadLetterEntry struct {
	Provider  string               `json:"provider"`
	Key       string               `json:"key"`
	Action    model.IncidentAction `json:"action"`
	Error     string               `json:"error"`
	Timestamp time.Time            `json:"timestamp"`
}

const channelCap = 256
const dlqCap = 100

const defaultMaxBackoff = 30 * time.Second

const (
	breakerThreshold = 3
	breakerCooldown  = 60 * time.Second
	maxAnalysisChars = 600
)

// localEndpoint is the in-pod sidecar address (loopback only).

const localEndpoint = "http://localhost:8080"

func (a *AlertManager) recordDeadLetter(entry *providerEntry, inc *model.Incident, action model.IncidentAction, err error) {
	a.dlqMu.Lock()
	defer a.dlqMu.Unlock()
	a.dlqRing[a.dlqHead] = DeadLetterEntry{
		Provider:  entry.provider.Name(),
		Key:       string(inc.Key),
		Action:    action,
		Error:     err.Error(),
		Timestamp: time.Now(),
	}
	a.dlqHead = (a.dlqHead + 1) % dlqCap
}

// enrichOne runs LLM enrichment for a single job, then fans out.
// Always fans out, even on panic (best-effort enrichment).

func (a *AlertManager) enrichOne(ctx context.Context, job deliverJob) {
	defer func() {
		a.mu.Lock()
		a.fanOut(job)
		a.mu.Unlock()
	}()
	defer func() {
		if r := recover(); r != nil {
			metrics.Default.LLMEnrichFailed.Add(1)
			klog.ErrorS(fmt.Errorf("%v", r), "llm enrichment panic recovered", "key", job.inc.Key)
		}
	}()
	// Creates and updates share the single FIFO enrich channel so an update
	// can never overtake its own create. Incidents that already carry analysis
	// (an earlier create/update succeeded) and obvious reasons skip the LLM
	// call entirely — they only pass through the queue for ordering.
	if job.inc.Analysis != "" || isObviousReason(job.inc.Reason) {
		return
	}
	if !a.brk.allow(time.Now()) {
		metrics.Default.LLMEnrichSkipped.Add(1)
		return
	}
	cctx, cancel := context.WithTimeout(ctx, llm.RequestTimeout)
	out, err := a.llm.Analyze(cctx, job.inc)
	cancel()
	a.brk.record(time.Now(), err == nil)
	metrics.Default.LLMEnrichTotal.Add(1)
	if err != nil {
		metrics.Default.LLMEnrichFailed.Add(1)
		klog.V(2).InfoS("llm enrichment skipped", "key", job.inc.Key, "error", err)
	} else if s := sanitizeAnalysis(out); s != "" {
		job.inc.Analysis = s
		if w := a.analysisWriter; w != nil {
			w(string(job.inc.Key), s)
		}
	}
}

func sanitizeAnalysis(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if len(s) > maxAnalysisChars {
		cut := maxAnalysisChars
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "…"
	}
	return s
}

// deliverOne handles the full send+retry for a single (entry, incident) pair.

func (a *AlertManager) deliverOne(ctx context.Context, entry *providerEntry, inc *model.Incident, action model.IncidentAction, ins *insight.Insight) {
	p := entry.provider
	metrics.Default.NotificationsTotal.Add(1)

	tpl := entry.templates
	if len(tpl) == 0 {
		tpl = a.templates
	}

	// Evaluate routes before rendering so route-filtered incidents don't pay
	// for message building (routes depend only on the incident, not the message).
	if !shouldDeliver(entry.routes, inc) {
		klog.V(4).InfoS("incident filtered by route",
			"provider", p.Name(),
			"key", inc.Key)
		return
	}

	raw := a.buildMessage(inc, action, ins, tpl)
	msg := truncateMsg(raw, entry.maxBytes)

	var err error
	if tp, ok := p.(ThreadProvider); ok {
		sendInc := inc
		if entry.maxBytes > 0 {
			sendInc = a.clampIncidentForProvider(inc, action, ins, entry.maxBytes, tpl, len(raw))
		}
		err = sendWithRetry(ctx, func() error {
			return tp.SendIncident(sendInc, action)
		}, entry.retry, p.Name())
	} else if _, ok := p.(EventDeliveryProvider); ok {
		ev := incidentToEvent(inc, action)
		err = sendWithRetry(ctx, func() error {
			return p.SendEvent(ev)
		}, entry.retry, p.Name())
	} else {
		err = sendWithRetry(ctx, func() error {
			return p.SendMessage(msg)
		}, entry.retry, p.Name())
	}
	if err != nil {
		metrics.Default.NotificationsDropped.Add(1)
		klog.ErrorS(err, "failed to send", "provider", p.Name(), "key", inc.Key, "id", inc.ID)
		a.recordDeadLetter(entry, inc, action, err)
		if entry.fallback != nil {
			fbMsg := truncateMsg("[fallback — primary "+p.Name()+" failed] "+msg, entry.fallback.maxBytes)
			fbErr := entry.fallback.provider.SendMessage(fbMsg)
			if fbErr != nil {
				klog.ErrorS(fbErr, "fallback delivery failed", "provider", entry.fallback.provider.Name())
			}
		}
	}
}

// buildMessage produces a formatted message string for the given incident.
// Uses the context-adaptive ReportBuilder and PlainTextRenderer.

func (a *AlertManager) fanOut(job deliverJob) {
	for _, entry := range a.entries {
		select {
		case entry.ch <- job:
		default:
			// Channel is saturated — drop the oldest queued job to make room
			// and record it in the dead-letter queue so a saturated provider
			// doesn't silently lose it.
			select {
			case dropped := <-entry.ch:
				metrics.Default.NotificationsDropped.Add(1)
				a.recordDeadLetter(&entry, dropped.inc, dropped.action, fmt.Errorf("delivery queue saturated"))
			default:
			}
			select {
			case entry.ch <- job:
			default:
				metrics.Default.NotificationsDropped.Add(1)
				a.recordDeadLetter(&entry, job.inc, job.action, fmt.Errorf("delivery queue saturated"))
			}
		}
	}
}

// deliverAllSync sends directly to every provider (synchronous).
// Used before Start() is called (e.g. kwatch replay).

func (a *AlertManager) deliverAllSync(inc *model.Incident, action model.IncidentAction, ins *insight.Insight) {
	for _, entry := range a.entries {
		p := entry.provider
		tpl := entry.templates
		if len(tpl) == 0 {
			tpl = a.templates
		}
		if !shouldDeliver(entry.routes, inc) {
			continue
		}
		raw := a.buildMessage(inc, action, ins, tpl)
		msg := truncateMsg(raw, entry.maxBytes)
		var err error
		if tp, ok := p.(ThreadProvider); ok {
			sendInc := inc
			if entry.maxBytes > 0 {
				sendInc = a.clampIncidentForProvider(inc, action, ins, entry.maxBytes, tpl, len(raw))
			}
			err = sendWithRetry(context.Background(), func() error {
				return tp.SendIncident(sendInc, action)
			}, entry.retry, p.Name())
		} else if _, ok := p.(EventDeliveryProvider); ok {
			ev := incidentToEvent(inc, action)
			err = sendWithRetry(context.Background(), func() error {
				return p.SendEvent(ev)
			}, entry.retry, p.Name())
		} else {
			err = sendWithRetry(context.Background(), func() error {
				return p.SendMessage(msg)
			}, entry.retry, p.Name())
		}
		if err != nil {
			metrics.Default.NotificationsDropped.Add(1)
			klog.ErrorS(err, "sync delivery failed", "provider", p.Name(), "key", inc.Key, "id", inc.ID)
		}
	}
}
