package health

import "time"

// HeartbeatComponent is the /health component of the heartbeat (the
// dead man's switch). It is always shown once a session starts, so a
// kwatch that tells nobody when it stops says so.
const HeartbeatComponent = "heartbeat"

// Heartbeat states (HeartbeatStatus.State). Only HeartbeatFailing is
// a degradation; the others are information and never turn /health
// red.
const (
	// HeartbeatProOnly: the Community edition has no heartbeat.
	HeartbeatProOnly = "pro_only"
	// HeartbeatNotConfigured: Pro, but heartbeatMonitor is off or has
	// no URL, so nobody is told when kwatch stops.
	HeartbeatNotConfigured = "not_configured"
	// HeartbeatWaiting: configured, and no ping was sent yet (pings
	// wait until monitoring is ready).
	HeartbeatWaiting = "waiting"
	// HeartbeatOK: the last ping was accepted.
	HeartbeatOK = "ok"
	// HeartbeatFailing: the last ping failed or was refused.
	HeartbeatFailing = "failing"
)

// Why the last ping failed (HeartbeatStatus.Failure).
const (
	// HeartbeatUnreachable: the monitoring URL did not answer.
	HeartbeatUnreachable = "unreachable"
	// HeartbeatRejected: it answered with a status other than 2xx.
	HeartbeatRejected = "rejected"
)

// HeartbeatStatus is what /health, /api/status and the kwatch page say
// about the heartbeat. It never carries the URL: it often holds a
// token.
type HeartbeatStatus struct {
	State string `json:"state"`
	// IntervalSeconds is the time between pings while configured.
	IntervalSeconds int `json:"intervalSeconds,omitempty"`
	// LastPing is when the monitoring URL last accepted a ping.
	LastPing time.Time `json:"lastPing,omitzero"`
	// LastFailure is when a ping last failed, Failure why, and
	// HTTPStatus the answer of a refused ping.
	LastFailure time.Time `json:"lastFailure,omitzero"`
	Failure     string    `json:"failure,omitempty"`
	HTTPStatus  int       `json:"httpStatus,omitempty"`
}

// heartbeatComponentOf is the /health component of a heartbeat state:
// running while pings are accepted, degraded while they fail, and an
// available "off" or "waiting" entry otherwise, so an unconfigured
// heartbeat is visible without counting as a failure.
func heartbeatComponentOf(state string) (string, string, bool) {
	switch state {
	case HeartbeatOK:
		return "running", "", true
	case HeartbeatFailing:
		return "degraded", "heartbeat_failed", false
	case HeartbeatWaiting:
		return "waiting", "", true
	case HeartbeatProOnly:
		return "off", "heartbeat_pro_only", true
	default:
		return "off", "heartbeat_not_configured", true
	}
}

// SetHeartbeat publishes the heartbeat's state and its component.
func (h *HealthServer) SetHeartbeat(status HeartbeatStatus) {
	h.heartbeat.Store(&status)
	state, reason, available := heartbeatComponentOf(status.State)
	h.SetComponentStatus(HeartbeatComponent, state, reason, available)
}

// Heartbeat is the last published heartbeat state; ok is false before
// any.
func (h *HealthServer) Heartbeat() (HeartbeatStatus, bool) {
	status := h.heartbeat.Load()
	if status == nil {
		return HeartbeatStatus{}, false
	}
	return *status, true
}

// heartbeatOrNil is the heartbeat for the /health body.
func (h *HealthServer) heartbeatOrNil() *HeartbeatStatus {
	status, ok := h.Heartbeat()
	if !ok {
		return nil
	}
	return &status
}
