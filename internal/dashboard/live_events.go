package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// LiveEvent is the data of a "ready" or "changed" event on
// /api/events: the list's version and, on "changed", the incidents
// that changed since the stream's last event. More says that more
// changed than IDs names: read everything shown again.
type LiveEvent struct {
	Version string   `json:"version"`
	IDs     []string `json:"ids,omitempty"`
	More    bool     `json:"more,omitempty"`
}

// eventsHandler serves GET /api/events, a Server-Sent Events stream:
// "ready" first, "changed" whenever incidents change and "ping" every
// liveHeartbeat. At most maxLiveConns streams are open at once, and
// maxClientLive from one client; a stream ends after liveMaxBeats
// heartbeats, when its sign-in stops being valid, when the client
// leaves or the server shuts down.
func (s *Server) eventsHandler(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	reader := s.sources.incidents.Load()
	if reader == nil {
		unavailable(w)
		return
	}
	client := s.liveClient(r)
	switch s.live.acquire(client) {
	case liveFull:
		w.Header().Set("Retry-After", "30")
		writeError(w, http.StatusServiceUnavailable,
			"too many live connections")
		return
	case liveClientFull:
		w.Header().Set("Retry-After", "30")
		writeError(w, http.StatusTooManyRequests,
			"too many live connections from this client")
		return
	}
	defer s.live.release(client)
	rc := http.NewResponseController(w)
	// The server's read timeout would end a long stream; a GET has no
	// body to wait for.
	_ = rc.SetReadDeadline(time.Time{})
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	st := liveStream{w: w, rc: rc, wall: s.live.wall}
	s.streamEvents(r, st)
}

// liveClient names who opened a stream, for the per-client limit: the
// session when the request has a valid one, else the client's address:
// a made-up cookie cannot buy a fresh allowance.
func (s *Server) liveClient(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" &&
		s.auth.sessions.valid(c.Value) {
		return "session " + c.Value
	}
	if client := s.auth.forwardedClient(r); client != "" {
		return "address " + client
	}
	return "address " + remoteHost(r)
}

// liveStream writes events and pushes them past any buffer.
type liveStream struct {
	w    http.ResponseWriter
	rc   *http.ResponseController
	wall func() time.Time
}

func (st liveStream) send(event string, data any) error {
	body, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_ = st.rc.SetWriteDeadline(st.wall().Add(liveWriteWait))
	if _, err := fmt.Fprintf(st.w, "event: %s\ndata: %s\n\n", event,
		body); err != nil {
		return err
	}
	return st.rc.Flush()
}

// checkLive compares the incidents if the engine ran since the last
// comparison.
func (s *Server) checkLive() {
	reader := s.sources.incidents.Load()
	if reader == nil {
		return
	}
	s.live.check(s.now.Now(), s.sources.lastScan.Load(),
		(*reader).Incidents, s.texts.redact)
}

// streamEvents runs one stream until it ends.
func (s *Server) streamEvents(r *http.Request, st liveStream) {
	s.checkLive()
	cur := s.live.state()
	_ = st.rc.SetWriteDeadline(st.wall().Add(liveWriteWait))
	if _, err := fmt.Fprintf(st.w, "retry: %s\n",
		strconv.Itoa(liveRetryMillis)); err != nil {
		return
	}
	if st.send("ready", LiveEvent{Version: cur.version}) != nil {
		return
	}
	beat := s.live.tick(liveHeartbeat)
	defer beat.stop()
	poll := s.live.tick(liveCheck)
	defer poll.stop()
	for beats := 0; ; {
		var err error
		select {
		case <-r.Context().Done():
			return
		case <-s.live.done:
			return
		case <-poll.C:
			s.checkLive()
		case <-cur.wake:
			cur, err = s.sendChanged(st, cur.seq)
		case <-beat.C:
			beats++
			if ok, _ := s.auth.check(r); !ok || beats >= liveMaxBeats {
				return
			}
			err = st.send("ping", struct{}{})
		}
		if err != nil {
			return
		}
	}
}

// sendChanged sends what changed after seq and returns the new state.
func (s *Server) sendChanged(st liveStream, seq uint64) (liveState,
	error) {
	cur := s.live.state()
	ids, more := s.live.since(seq)
	if len(ids) == 0 && !more {
		return cur, nil
	}
	return cur, st.send("changed", LiveEvent{Version: cur.version,
		IDs: ids, More: more})
}
