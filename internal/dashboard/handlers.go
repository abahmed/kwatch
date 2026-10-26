package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"k8s.io/klog/v2"

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

func matches(p IncidentSnapshot, state, namespace string) bool {
	if state != "" {
		isResolved := p.State == "resolved"
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
	if namespace != "" && p.Root != namespace {
		return false
	}
	return true
}

func toListItem(p IncidentSnapshot) IncidentListItem {
	return IncidentListItem{
		ID:          p.ID,
		Title:       titleFor(p),
		Severity:    p.Tier,
		State:       p.State,
		Namespace:   namespaceFor(p.Root),
		Workload:    nameFor(p.Root),
		FirstSeen:   p.Opened,
		LastSeen:    lastSeen(p),
		RepeatCount: p.RepeatCount,
		AckedBy:     ackedBy(p),
		ResolvedAt:  p.Resolved,
	}
}

func toDetail(p IncidentSnapshot) IncidentDetail {
	return IncidentDetail{
		ID:               p.ID,
		Title:            titleFor(p),
		Severity:         p.Tier,
		State:            p.State,
		Namespace:        namespaceFor(p.Root),
		Workload:         nameFor(p.Root),
		RootCause:        p.CauseText,
		SuggestedFix:     fixFor(p),
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

func titleFor(p IncidentSnapshot) string {
	if p.Root != "" {
		return p.Root
	}
	return p.ID
}

func namespaceFor(root string) string {
	parts := strings.Split(root, "/")
	if len(parts) == 2 {
		return parts[0]
	}
	return ""
}

func nameFor(root string) string {
	parts := strings.Split(root, "/")
	if len(parts) == 2 {
		return parts[1]
	}
	return root
}

func lastSeen(p IncidentSnapshot) time.Time {
	if !p.Resolved.IsZero() {
		return p.Resolved
	}
	if !p.Announced.IsZero() {
		return p.Announced
	}
	return p.Opened
}

func ackedBy(p IncidentSnapshot) string {
	if p.Ack != nil {
		return p.Ack.By
	}
	return ""
}

func fixFor(p IncidentSnapshot) string {
	if p.Fix.Change != "" {
		return p.Fix.Change
	}
	return ""
}

func toTimeline(events []TimelineEvent) []Event {
	out := make([]Event, 0, len(events))
	for _, e := range events {
		out = append(out, Event{
			At:     e.At,
			Text:   e.Text,
			Entity: e.Entity,
		})
	}
	return out
}
