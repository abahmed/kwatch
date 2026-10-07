package health

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/status"
)

// maxStatusBytes bounds a /status response.
const maxStatusBytes = 64 << 10

// statusSource builds the report from the live pipeline.
type statusSource func() status.Report

// statusHolder keeps the current source; the zero value has none.
type statusHolder struct{ source atomic.Pointer[statusSource] }

// SetStatusSource sets where /status gets its report: the running
// pipeline. Without one, or after nil, /status answers 503.
func (h *HealthServer) SetStatusSource(source func() status.Report) {
	if source == nil {
		h.statusHolder.source.Store(nil)
		return
	}
	s := statusSource(source)
	h.statusHolder.source.Store(&s)
}

// statusHandler serves /status, read-only: plain text by default and
// JSON with ?format=json. It reads state kwatch already holds; it
// shows the names of objects to anyone who can reach the health port.
func (h *HealthServer) statusHandler(
	w http.ResponseWriter, r *http.Request,
) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	source := h.statusHolder.source.Load()
	if source == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		writeBody(w, []byte("status unavailable: kwatch is not "+
			"monitoring a cluster right now\n"))
		return
	}
	report := (*source)()
	report.Gaps = h.coverageGaps()
	body, contentType := renderStatus(report, r.URL.Query().Get("format"))
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	writeBody(w, body)
}

// renderStatus writes the report as JSON or text, within the bound.
func renderStatus(report status.Report, format string) ([]byte, string) {
	if format == "json" {
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil || len(body) > maxStatusBytes {
			return []byte("{\"error\":\"status too large\"}\n"),
				"application/json"
		}
		return append(body, '\n'), "application/json"
	}
	text := report.Text()
	if len(text) > maxStatusBytes {
		cut := strings.LastIndexByte(text[:maxStatusBytes], '\n')
		text = text[:cut+1] + "... cut: status too large\n"
	}
	return []byte(text), "text/plain; charset=utf-8"
}

func writeBody(w http.ResponseWriter, body []byte) {
	if _, err := w.Write(body); err != nil {
		klog.ErrorS(err, "health: write status response")
	}
}
