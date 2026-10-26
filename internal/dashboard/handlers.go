package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/version"
)

// incidentsHandler serves GET /api/incidents?state=active|resolved&namespace=.
func (s *Server) incidentsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")

	state := r.URL.Query().Get("state")
	namespace := r.URL.Query().Get("namespace")

	items := s.incidents.Incidents()
	list := make([]IncidentListItem, 0, len(items))
	for _, p := range items {
		if !matches(p, state, namespace) {
			continue
		}
		list = append(list, toListItem(p))
	}
	body, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	body = append(body, '\n')
	if _, err := w.Write(body); err != nil {
		klog.ErrorS(err, "dashboard: write incidents response")
	}
}

// incidentDetailHandler serves GET /api/incidents/{id}.
func (s *Server) incidentDetailHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")

	id := strings.TrimPrefix(r.URL.Path, "/api/incidents/")
	if id == "" || id == "/" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	p := s.incidents.Incident(id)
	if p == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	body, err := json.MarshalIndent(toDetail(*p), "", "  ")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	body = append(body, '\n')
	if _, err := w.Write(body); err != nil {
		klog.ErrorS(err, "dashboard: write incident detail response")
	}
}

// statusHandler serves GET /api/status.
func (s *Server) statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")

	report := s.status()
	resp := StatusResponse{
		Version:           version.Short(),
		Edition:           s.edition,
		LastScan:          report.At,
		WatchedNamespaces: nil,
		PerChannelHealth:  nil,
	}
	body, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	body = append(body, '\n')
	if _, err := w.Write(body); err != nil {
		klog.ErrorS(err, "dashboard: write status response")
	}
}

func matches(p incident.Incident, state, namespace string) bool {
	if state != "" {
		isResolved := p.State == incident.Resolved
		switch state {
		case "active":
			if isResolved {
				return false
			}
		case "resolved":
			if !isResolved {
				return false
			}
		default:
			return false
		}
	}
	if namespace != "" && p.Root.Namespace != namespace {
		return false
	}
	return true
}

func toListItem(p incident.Incident) IncidentListItem {
	return IncidentListItem{
		ID:          p.ID,
		Title:       titleFor(p),
		Severity:    p.Tier.String(),
		State:       p.State.String(),
		Namespace:   p.Root.Namespace,
		Workload:    p.Root.Name,
		FirstSeen:   p.Opened,
		LastSeen:    lastSeen(p),
		RepeatCount: p.RepeatCount,
		AckedBy:     ackedBy(p),
		ResolvedAt:  p.Resolved,
	}
}

func toDetail(p incident.Incident) IncidentDetail {
	return IncidentDetail{
		ID:               p.ID,
		Title:            titleFor(p),
		Severity:         p.Tier.String(),
		State:            p.State.String(),
		Namespace:        p.Root.Namespace,
		Workload:         p.Root.Name,
		RootCause:        causeText(p),
		SuggestedFix:     fixText(p),
		FirstSeen:        p.Opened,
		LastSeen:         lastSeen(p),
		RepeatCount:      p.RepeatCount,
		ResolvedAt:       p.Resolved,
		Timeline:         toTimeline(p.Timeline),
		KubernetesEvents: nil,
		CapturedLogLines: nil,
		DeliveryAttempts: nil,
		DocsLink:         "",
	}
}

func titleFor(p incident.Incident) string {
	if p.Root.Name != "" {
		return p.Root.String()
	}
	return p.ID
}

func lastSeen(p incident.Incident) time.Time {
	if !p.Resolved.IsZero() {
		return p.Resolved
	}
	if !p.Announced.IsZero() {
		return p.Announced
	}
	return p.Opened
}

func ackedBy(p incident.Incident) string {
	if p.Ack != nil {
		return p.Ack.On.String()
	}
	return ""
}

func causeText(p incident.Incident) string {
	if p.Cause != nil {
		return p.Cause.Summary
	}
	return ""
}

func fixText(p incident.Incident) string {
	return string(p.Fix)
}

func toTimeline(events []incident.Event) []Event {
	out := make([]Event, 0, len(events))
	for _, e := range events {
		out = append(out, Event{
			At:     e.At,
			Text:   e.Text,
			Entity: entityIDString(e.Entity),
		})
	}
	return out
}

func entityIDString(e *inventory.EntityID) string {
	if e == nil {
		return ""
	}
	return e.String()
}
