package model

import "time"

type ContainerState struct {
	RestartCount     int32
	LastTerminatedOn time.Time
	Reason           string
	Msg              string
	ExitCode         int32
	Status           string
}

type IncidentAction int

const (
	ActionCreate IncidentAction = iota
	ActionUpdate
	ActionSkip
	ActionStale
	ActionResolved
	ActionDigest
	ActionDigestFlush
)

func (a IncidentAction) String() string {
	switch a {
	case ActionCreate:
		return "create"
	case ActionUpdate:
		return "update"
	case ActionSkip:
		return "skip"
	case ActionStale:
		return "stale"
	case ActionResolved:
		return "resolved"
	case ActionDigest:
		return "digest"
	case ActionDigestFlush:
		return "digest_flush"
	default:
		return "unknown"
	}
}

type IncidentState int

const (
	StateActive IncidentState = iota
	StateStale
	StateResolved
	StatePendingResolve
)

type IncidentView struct {
	Key       string         `json:"key"`
	Reason    string         `json:"reason"`
	Namespace string         `json:"namespace"`
	Name      string         `json:"name"`
	State     IncidentState  `json:"state"`
	Severity  string         `json:"severity"`
	Count     int            `json:"count"`
	FirstSeen time.Time      `json:"firstSeen"`
	LastSeen  time.Time      `json:"lastSeen"`
	Hint      string         `json:"hint,omitempty"`
}

type Incident struct {
	Key           string
	Reason        string
	Namespace     string
	Resource      string
	Name          string
	Count         int
	FirstSeen     time.Time
	LastSeen      time.Time
	Resources     map[string]bool
	OwnerKind     string
	ContainerName string
	RestartCount  int
	Hint          string
	Logs          string
	Events        string
	State         IncidentState
	LastUpdate    time.Time
	LastContainerState *ContainerState
	Severity           string
	SuppressedPods     int
	ResolveAt          time.Time
}
