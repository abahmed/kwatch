package dashboard

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/version"
)

// maxResponseBytes bounds a dashboard API response, mirroring
// maxStatusBytes on /status.
const maxResponseBytes = 64 << 10

// List limits: the default and the largest page of /api/incidents.
const (
	defaultListLimit = 100
	maxListLimit     = 500
)

// listEnvelopeBytes is room kept for the list's envelope around items.
const listEnvelopeBytes = 128

// writeJSON writes v as JSON. A body over maxResponseBytes is refused
// with 500 rather than cut; callers bound what they pass.
func writeJSON(w http.ResponseWriter, v any) {
	body, err := json.Marshal(v)
	if err != nil || len(body) > maxResponseBytes {
		klog.ErrorS(err, "dashboard: response not written",
			"bytes", len(body))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(append(body, '\n'))
}

// writeError writes {"error": msg} with code.
func writeError(w http.ResponseWriter, code int, msg string) {
	body, _ := json.Marshal(map[string]string{"error": msg})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(append(body, '\n'))
}

// onlyGet answers 405 to anything but GET and reports whether to go on.
func onlyGet(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	w.Header().Set("Allow", http.MethodGet)
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	return false
}

// unavailable answers 503: the pipeline is not running.
func unavailable(w http.ResponseWriter) {
	writeError(w, http.StatusServiceUnavailable, "unavailable")
}

// incidentsHandler serves
// GET /api/incidents?state=active|resolved&namespace=&limit=, newest
// first.
func (s *Server) incidentsHandler(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	q := r.URL.Query()
	f := filter{state: q.Get("state"), namespace: q.Get("namespace")}
	if !f.validState() {
		writeError(w, http.StatusBadRequest, "invalid state")
		return
	}
	limit, ok := parseLimit(q.Get("limit"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	reader := s.sources.incidents.Load()
	if reader == nil {
		unavailable(w)
		return
	}
	items := make([]IncidentListItem, 0)
	for _, p := range (*reader).Incidents() {
		if f.matches(p) {
			items = append(items, toListItem(p))
		}
	}
	slices.SortFunc(items, newestFirst)
	writeJSON(w, boundList(items, limit))
}

// parseLimit reads ?limit=: empty is the default, larger than the
// maximum is the maximum, anything not a positive number is invalid.
func parseLimit(raw string) (int, bool) {
	if raw == "" {
		return defaultListLimit, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, false
	}
	return min(n, maxListLimit), true
}

// boundList keeps at most limit items and as many as fit the response
// size, newest first; Truncated says some were left out.
func boundList(items []IncidentListItem, limit int) ListResponse {
	out := ListResponse{Incidents: make([]IncidentListItem, 0),
		Total: len(items)}
	budget := maxResponseBytes - listEnvelopeBytes
	for _, item := range items {
		if len(out.Incidents) == limit {
			break
		}
		body, err := json.Marshal(item)
		if err != nil || len(body)+1 > budget {
			break
		}
		budget -= len(body) + 1
		out.Incidents = append(out.Incidents, item)
	}
	out.Truncated = len(out.Incidents) < len(items)
	return out
}

// incidentDetailHandler serves GET /api/incidents/{id}.
func (s *Server) incidentDetailHandler(
	w http.ResponseWriter, r *http.Request,
) {
	if !onlyGet(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/incidents/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	reader := s.sources.incidents.Load()
	if reader == nil {
		unavailable(w)
		return
	}
	p := (*reader).Incident(id)
	if p == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, toDetail(*p))
}

// statusHandler serves GET /api/status.
func (s *Server) statusHandler(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	src := s.sources.status.Load()
	if src == nil {
		unavailable(w)
		return
	}
	health := make([]ChannelHealth, 0)
	if h := s.sources.health.Load(); h != nil {
		health = append(health, (*h)()...)
	}
	writeJSON(w, StatusResponse{
		Version:            version.Short(),
		Edition:            s.edition,
		LastScan:           (*src)().At,
		WatchedNamespaces:  watched(s.scope.Watched),
		ExcludedNamespaces: s.scope.Excluded,
		PerChannelHealth:   health,
	})
}

// allNamespaces is what watchedNamespaces says when every namespace is
// watched. "*" is never a valid namespace name.
const allNamespaces = "*"

func watched(names []string) []string {
	if len(names) == 0 {
		return []string{allNamespaces}
	}
	return names
}
