package dashboard

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"

	"github.com/abahmed/kwatch/internal/delivery/webpush"
)

// maxPushBody bounds a push request body: one subscription.
const maxPushBody = 4 << 10

// pushKey is GET /api/push: the key a browser subscribes with.
type pushKey struct {
	PublicKey string `json:"publicKey"`
}

// pushStatus is POST /api/push/status: how one browser stands. The
// push address is a secret of the browser's, so it travels in a body,
// never in a URL a proxy might log.
type pushStatus struct {
	Subscribed bool `json:"subscribed"`
	// Failing says why the last notification to this browser failed.
	Failing string `json:"failing,omitempty"`
}

// pushRequest is POST /api/push: a browser's subscription and what it
// wants told. Replaces is the address of the subscription the browser
// had before it renewed it (the service worker's
// pushsubscriptionchange); with it, an empty Min keeps the old
// settings and the old address is forgotten.
type pushRequest struct {
	Subscription webpush.Subscription `json:"subscription"`
	Min          string               `json:"min"`
	Resolved     bool                 `json:"resolved"`
	Replaces     string               `json:"replaces,omitempty"`
}

// pushHandler serves GET /api/push (the key) and POST /api/push
// (subscribe, or change what to tell): 204, 400 for a subscription
// kwatch can't send to, 503 while the pipeline is not running.
func (s *Server) pushHandler(w http.ResponseWriter, r *http.Request) {
	p := s.sources.push.Load()
	switch r.Method {
	case http.MethodGet:
		if p == nil {
			unavailable(w)
			return
		}
		writeJSON(w, r, pushKey{PublicKey: p.book.key.Public()})
	case http.MethodPost:
		var req pushRequest
		if !readPushBody(w, r, &req) {
			return
		}
		if !req.Subscription.Valid() {
			writeError(w, http.StatusBadRequest,
				"not a subscription kwatch can send to")
			return
		}
		if p == nil {
			unavailable(w)
			return
		}
		prefs := pushPrefs{Min: req.Min, Resolved: req.Resolved}
		if old, ok := p.book.prefsOf(req.Replaces); ok && req.Min == "" {
			prefs = old
		}
		if _, ok := pushRank[prefs.Min]; !ok {
			writeError(w, http.StatusBadRequest,
				"min must be page, notify or digest")
			return
		}
		if err := p.book.subscribe(req.Subscription, prefs,
			req.Replaces, s.now.Now()); err != nil {
			writeError(w, http.StatusServiceUnavailable,
				"could not save the subscription")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// pushStatusHandler serves POST /api/push/status {endpoint}.
func (s *Server) pushStatusHandler(w http.ResponseWriter, r *http.Request) {
	var req pushEndpoint
	if !onlyPost(w, r) || !readPushBody(w, r, &req) {
		return
	}
	p := s.sources.push.Load()
	if p == nil {
		unavailable(w)
		return
	}
	on, failing := p.book.status(req.Endpoint)
	writeJSON(w, r, pushStatus{Subscribed: on, Failing: failing})
}

// pushEndpoint is the body of POST /api/push/status, /remove and
// /test.
type pushEndpoint struct {
	Endpoint string `json:"endpoint"`
}

// pushRemoveHandler serves POST /api/push/remove: the browser turned
// notifications off or signed out. 204 also when it was not known.
func (s *Server) pushRemoveHandler(w http.ResponseWriter, r *http.Request) {
	var req pushEndpoint
	if !onlyPost(w, r) || !readPushBody(w, r, &req) {
		return
	}
	p := s.sources.push.Load()
	if p == nil {
		unavailable(w)
		return
	}
	if err := p.book.remove(req.Endpoint); err != nil {
		writeError(w, http.StatusServiceUnavailable,
			"could not save the change")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pushTestHandler serves POST /api/push/test: one notification to a
// subscribed browser now. 204 when its push service took it, 404 for
// a browser that is not subscribed, 502 with the reason otherwise.
func (s *Server) pushTestHandler(w http.ResponseWriter, r *http.Request) {
	var req pushEndpoint
	if !onlyPost(w, r) || !readPushBody(w, r, &req) {
		return
	}
	p := s.sources.push.Load()
	if p == nil {
		unavailable(w)
		return
	}
	sub, ok := p.book.find(req.Endpoint)
	if !ok {
		writeError(w, http.StatusNotFound, "not subscribed")
		return
	}
	if err := p.test(r.Context(), sub, s.now.Now()); err != nil {
		writeError(w, http.StatusBadGateway, pushFailure(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// onlyPost answers 405 to anything but POST and reports whether to go
// on.
func onlyPost(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodPost {
		return true
	}
	w.Header().Set("Allow", http.MethodPost)
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	return false
}

// readPushBody reads a JSON body into v. The request must come from
// the dashboard's own page (Origin) as JSON, which no form on another
// site can send; it answers 400 or 403 itself and reports false.
func readPushBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "cross-origin request")
		return false
	}
	ct, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || ct != "application/json" {
		writeError(w, http.StatusBadRequest, "send JSON")
		return false
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxPushBody+1))
	if err != nil || len(body) > maxPushBody ||
		json.Unmarshal(body, v) != nil {
		writeError(w, http.StatusBadRequest, "bad request body")
		return false
	}
	return true
}
