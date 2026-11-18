package dashboard

import "github.com/abahmed/kwatch/internal/health"

// heartbeatSource is the heartbeat's state; ok is false before any.
type heartbeatSource func() (health.HeartbeatStatus, bool)

// SetHeartbeat sets the source of the heartbeat's state; nil leaves
// heartbeat out of the kwatch part of /api/status.
func (s *Server) SetHeartbeat(src func() (health.HeartbeatStatus, bool)) {
	if src == nil {
		s.sources.heartbeat.Store(nil)
		return
	}
	v := heartbeatSource(src)
	s.sources.heartbeat.Store(&v)
}

// heartbeat is the heartbeat's state for /api/status; nil without a
// source or before a session published it.
func (s *Server) heartbeat() *health.HeartbeatStatus {
	src := s.sources.heartbeat.Load()
	if src == nil {
		return nil
	}
	status, ok := (*src)()
	if !ok {
		return nil
	}
	return &status
}
