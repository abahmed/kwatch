package enricher

import (
	"strings"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

var defaultSeverityByOwnerKind = map[string]string{
	"StatefulSet": "high",
}

var defaultSeverityByReason = map[string]string{
	constant.ReasonEvicted:          "medium",
	constant.ReasonImagePullBackOff: "medium",
}

type Enricher interface {
	Enrich(ev *event.Event, inc *model.Incident)
}

type DefaultEnricher struct {
	SeverityByOwnerKind map[string]string
	SeverityByReason    map[string]string
}

func (e *DefaultEnricher) SetSeverityMap(m map[string]string) {
	e.SeverityByOwnerKind = m
}

func (e *DefaultEnricher) Enrich(ev *event.Event, inc *model.Incident) {
	inc.OwnerKind = ev.OwnerKind
	if ev.ContainerName != "" && ev.ContainerName != "." {
		inc.ContainerName = ev.ContainerName
	}
	if ev.Image != "" {
		inc.Image = ev.Image
	}
	if ev.NodeName != "" {
		inc.NodeName = ev.NodeName
	}
	if ev.Hint != "" {
		inc.Hint = ev.Hint
	} else {
		inc.Hint = hintForReason(ev.Reason)
	}
	// CD-3: signature-based hints for common patterns
	if sh := SignatureHint(ev.Logs); sh != "" {
		inc.Hint = combineHints(inc.Hint, sh)
	}
	inc.Logs = ev.Logs
	inc.Events = ev.Events
	inc.IncludeEvents = ev.IncludeEvents
	inc.IncludeLogs = ev.IncludeLogs
	newSev := ev.Severity
	if newSev == "" {
		newSev = e.resolveSeverity(ev.OwnerKind, ev.Reason)
	}
	// severity is strictly monotonic (sticky escalation): once raised, it never
	// downgrades until the incident resolves. This is intentional — a runtime
	// config change via CRD watcher that lowers SeverityByReason or
	// SeverityByOwnerKind will not take effect on already-open incidents.
	if newSev.Rank() >= inc.Severity.Rank() {
		inc.Severity = newSev
	}
}

func (e *DefaultEnricher) resolveSeverity(ownerKind, reason string) model.Severity {
	if e.SeverityByReason != nil {
		if s, ok := lookupCaseInsensitive(e.SeverityByReason, reason); ok {
			return model.SeverityFromString(s)
		}
	}
	if s, ok := defaultSeverityByReason[reason]; ok {
		return model.SeverityFromString(s)
	}
	if e.SeverityByOwnerKind != nil {
		if s, ok := lookupCaseInsensitive(e.SeverityByOwnerKind, ownerKind); ok {
			return model.SeverityFromString(s)
		}
		if s, ok := e.SeverityByOwnerKind["default"]; ok {
			return model.SeverityFromString(s)
		}
	}
	if s, ok := defaultSeverityByOwnerKind[ownerKind]; ok {
		return model.SeverityFromString(s)
	}
	return model.SeverityNormal
}

// lookupCaseInsensitive matches a map key regardless of case. K8s kinds and
// reasons arrive canonically capitalized while user config keys may use any
// casing.
func lookupCaseInsensitive(m map[string]string, key string) (string, bool) {
	if v, ok := m[key]; ok {
		return v, true
	}
	lk := strings.ToLower(key)
	for k, v := range m {
		if strings.ToLower(k) == lk {
			return v, true
		}
	}
	return "", false
}
