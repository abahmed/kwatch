package dashboard

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/version"
)

// maxResponseBytes bounds a dashboard API response, mirroring
// maxStatusBytes on /status.
const maxResponseBytes = 64 << 10

// writeJSON writes v as JSON. A body over maxResponseBytes is refused
// with 500 rather than cut; callers bound what they pass. The log names
// the request path so a refusal can be traced to its endpoint.
func writeJSON(w http.ResponseWriter, r *http.Request, v any) {
	body, err := encodeBody(v)
	if err != nil {
		klog.ErrorS(err, "dashboard: response not written",
			"path", r.URL.Path, "bytes", len(body))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(append(body, '\n'))
}

// encodeBody marshals v and fails when the body is over the limit.
func encodeBody(v any) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode response: %w", err)
	}
	if len(body)+1 > maxResponseBytes {
		return body, fmt.Errorf("response body of %d bytes is over "+
			"the %d byte limit", len(body)+1, maxResponseBytes)
	}
	return body, nil
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

// incidentsHandler serves GET /api/incidents: one page of the list
// parseListRequest describes. The answer carries an ETag; a poll that
// sends it back in If-None-Match while nothing changed gets 304.
func (s *Server) incidentsHandler(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	now := s.now.Now()
	req, err := parseListRequest(r.URL.Query(), now)
	if err != nil {
		writeBadQuery(w, err)
		return
	}
	reader := s.sources.incidents.Load()
	if reader == nil {
		unavailable(w)
		return
	}
	items := (*reader).Incidents()
	if req.filter.nodes != nil {
		var model inventory.Reader
		if m := s.sources.model.Load(); m != nil {
			model = *m
		}
		req.filter.onNodes = onNodes(model, items, req.filter.nodes)
	}
	if req.filter.wrongReleases {
		req.filter.onWrong = s.wrongReleaseIssues(now)
	}
	writeTagged(w, r, buildList(req, items, now, s.texts.redact))
}

// writeTagged writes v as JSON with an ETag of its body, or 304 when
// the request already holds that body.
func writeTagged(w http.ResponseWriter, r *http.Request, v any) {
	body, err := encodeBody(v)
	if err != nil {
		klog.ErrorS(err, "dashboard: response not written",
			"path", r.URL.Path, "bytes", len(body))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	tag := bodyETag(body)
	w.Header().Set("ETag", tag)
	if etagMatches(r.Header.Get("If-None-Match"), tag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(append(body, '\n'))
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
	writeJSON(w, r, s.detailOf(*p))
}

// statusHandler serves GET /api/status.
func (s *Server) statusHandler(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	if s.sources.status.Load() == nil {
		unavailable(w)
		return
	}
	health := make([]ChannelHealth, 0)
	if h := s.sources.health.Load(); h != nil {
		for _, c := range (*h)() {
			c.Type = cmp.Or(c.Type, c.Channel)
			c.Name = channelName(c.Type)
			health = append(health, c)
		}
	}
	last := s.lastScan()
	writeJSON(w, r, StatusResponse{
		Version:            version.Short(),
		Edition:            s.opts.Edition,
		LastScan:           last,
		LastScanLate:       !last.IsZero() && s.now.Now().Sub(last) > scanLate,
		KwatchNamespace:    s.opts.Self.Namespace,
		KwatchDeployment:   s.opts.Self.Deployment,
		WatchedNamespaces:  watched(s.opts.Scope.Watched),
		ExcludedNamespaces: s.opts.Scope.Excluded,
		PerChannelHealth:   health,
		Self:               s.selfStatus(),
		FailedLastHour:     s.failedLastHour(),
	})
}

// A scan older than scanLate means kwatch is not checking: the engine
// reports a loop iteration at least every 10 seconds.
const scanLate = 2 * time.Minute

func (s *Server) lastScan() time.Time {
	n := s.sources.lastScan.Load()
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n).UTC()
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
