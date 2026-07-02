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
	StateResolved
	StatePendingResolve
)

type IncidentView struct {
	Key       string        `json:"key"`
	Reason    string        `json:"reason"`
	Namespace string        `json:"namespace"`
	Name      string        `json:"name"`
	State     IncidentState `json:"state"`
	Severity  string        `json:"severity"`
	Count     int           `json:"count"`
	FirstSeen time.Time     `json:"firstSeen"`
	LastSeen  time.Time     `json:"lastSeen"`
	Hint      string        `json:"hint,omitempty"`
	Analysis  string        `json:"analysis,omitempty"`
}

type Incident struct {
	ID                 string // stable short hash for log correlation
	Key                string
	Reason             string
	Namespace          string
	Resource           string
	Name               string
	Count              int
	FirstSeen          time.Time
	LastSeen           time.Time
	Resources          map[string]bool
	PeakResources      int
	Containers         map[string]bool
	OwnerKind          string
	ContainerName      string
	RestartCount       int
	Hint               string
	Analysis           string
	Runbook            string
	Logs               string
	Events             string
	State              IncidentState
	LastUpdate         time.Time
	LastContainerState *ContainerState
	Severity           string
	SuppressedPods     int
	SuppressedOwners   map[string]int // owner → count of suppressed pods
	ResolveAt          time.Time
	IncludeEvents      bool
	IncludeLogs        bool
	NodeName           string
	NotifiedSig        string
	LastNotifiedAt     time.Time
	RenotifyCount      int
	Digested           bool // created via storm digest; suppress resolve/renotify edge
}

// Clone returns a deep copy of the incident, safe for concurrent use.
func (inc *Incident) Clone() *Incident {
	c := *inc
	c.Resources = make(map[string]bool, len(inc.Resources))
	for k, v := range inc.Resources {
		c.Resources[k] = v
	}
	c.Containers = make(map[string]bool, len(inc.Containers))
	for k, v := range inc.Containers {
		c.Containers[k] = v
	}
	if inc.LastContainerState != nil {
		cs := *inc.LastContainerState
		c.LastContainerState = &cs
	}
	if inc.SuppressedOwners != nil {
		c.SuppressedOwners = make(map[string]int, len(inc.SuppressedOwners))
		for k, v := range inc.SuppressedOwners {
			c.SuppressedOwners[k] = v
		}
	}
	return &c
}
