package health

import (
	"time"

	"github.com/abahmed/kwatch/internal/status"
)

// PodLogsComponent is the health component of pod log reads: degraded
// with optional_permission_denied while the API refuses pods/log, so
// crash output is not shown. It never raises an incident.
const PodLogsComponent = "pod-logs"

// LinkStatus is whether messages link to the dashboard. Reason, set
// when they do not, is one of no_route, port_mismatch,
// unusable_address and routes_forbidden.
type LinkStatus struct {
	On      bool   `json:"on"`
	Address string `json:"address,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// SelfStop is how one kwatch session ended: when the next one started
// (At) and why the previous one stopped (Reason, a restart reason code:
// graceful_shutdown, leader_handoff, internal_failure, oom_killed,
// eviction, node_disruption, deployment_rollout, api_unavailable or
// unknown). StoppedAt is when it stopped, zero when unknown (a crash
// leaves no stop record).
type SelfStop struct {
	At        time.Time `json:"at"`
	Reason    string    `json:"reason"`
	StoppedAt time.Time `json:"stoppedAt,omitzero"`
}

// StartupProgress is how far the first sync of the watches is.
type StartupProgress struct {
	// Kinds, Synced and Settled count the typed watches, the ones
	// whose initial list is in, and the ones no longer holding up
	// startup; Required and RequiredSynced the ones the first alerts
	// wait for.
	Kinds          int `json:"kinds"`
	Synced         int `json:"synced"`
	Settled        int `json:"settled"`
	Required       int `json:"required"`
	RequiredSynced int `json:"requiredSynced"`
	// Objects counts the objects listed so far.
	Objects int64 `json:"objects"`
	// DynamicWatched and DynamicSynced count the discovered kinds
	// watched and synced.
	DynamicWatched int `json:"dynamicWatched"`
	DynamicSynced  int `json:"dynamicSynced"`
	// Done is set once every typed kind settled.
	Done bool `json:"done"`
}

// SelfReport is kwatch's view of itself, for the dashboard overview.
type SelfReport struct {
	StartedAt time.Time `json:"startedAt"`
	// LastStop is how the previous session ended; nil on a first run.
	LastStop *SelfStop `json:"lastStop,omitempty"`
	// Restarts are the starts of the last 24 hours that followed an
	// earlier session, newest first.
	Restarts []SelfStop `json:"restarts"`
	// LastAPIContact is the last successful Lease renewal: kwatch
	// reached the API server then. Zero without leader election.
	LastAPIContact time.Time `json:"lastApiContact,omitzero"`
	// StateFileBytes is the state file's data size.
	StateFileBytes int64 `json:"stateFileBytes"`
	// OutboxPending counts the messages accepted but not yet sent.
	OutboxPending int `json:"outboxPending"`
	// DashboardLinks says whether messages link to the dashboard.
	DashboardLinks LinkStatus `json:"dashboardLinks"`
	// PodLogsForbidden is set while kwatch may not read pod logs:
	// crash output is not shown.
	PodLogsForbidden bool `json:"podLogsForbidden"`
	// Startup is the first sync's progress; nil before the sources
	// exist.
	Startup *StartupProgress `json:"startup,omitempty"`
}

// SetDashboardLinks publishes whether messages link to the dashboard,
// for /status.
func (h *HealthServer) SetDashboardLinks(links LinkStatus) {
	h.links.Store(&links)
}

// DashboardLinks is the last published link state; ok is false before
// any.
func (h *HealthServer) DashboardLinks() (LinkStatus, bool) {
	links := h.links.Load()
	if links == nil {
		return LinkStatus{}, false
	}
	return *links, true
}

// linkGap is the /status line for dashboard links that are off.
func (h *HealthServer) linkGap() (status.Gap, bool) {
	links, ok := h.DashboardLinks()
	if !ok || links.On {
		return status.Gap{}, false
	}
	return status.Gap{What: "dashboard links in alerts",
		Reason: "off: " + linkReasons[links.Reason]}, true
}

// linkReasons word the reasons dashboard links are off.
var linkReasons = map[string]string{
	"no_route": "no Ingress or HTTPRoute in kwatch's namespace " +
		"sends to its dashboard Service",
	"port_mismatch": "a route reaches the dashboard Service but " +
		"not its \"dashboard\" port",
	"unusable_address": "the route's host is a wildcard or empty, " +
		"or its path is a pattern",
	"routes_forbidden": "kwatch may not read Ingresses or HTTPRoutes",
}
