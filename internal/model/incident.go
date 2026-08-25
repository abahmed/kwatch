package model

import (
	"time"
)

type ContainerState struct {
	RestartCount     int32
	LastTerminatedOn time.Time
	Reason           string
	Msg              string
	ExitCode         int32
	Status           string
	LastAlertAt      time.Time
}

// PodSummary is a lightweight representation of a suppressed pod,
// attached to the parent node incident for context.
type PodSummary struct {
	Namespace    string
	PodName      string
	Reason       string
	RestartCount int
}

type IncidentAction int

const (
	ActionCreate IncidentAction = iota
	ActionUpdate
	ActionSkip
	ActionResolved
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
	Key       IncidentKey   `json:"key"`
	Reason    string        `json:"reason"`
	Namespace string        `json:"namespace"`
	Name      string        `json:"name"`
	State     IncidentState `json:"state"`
	Severity  Severity      `json:"severity"`
	Count     int           `json:"count"`
	FirstSeen time.Time     `json:"firstSeen"`
	LastSeen  time.Time     `json:"lastSeen"`
	Hint      string        `json:"hint,omitempty"`
}

type Incident struct {
	ID        string // stable short hash for log correlation
	Key       IncidentKey
	Reason    string
	Namespace string
	Resource  string
	// Name identifies the subject of the incident; its encoding depends on the
	// resource kind, so compare via ParseKey/OwnerPath rather than raw equality:
	//   - pod incidents: bare owning-workload name, or the pod's own name when
	//     the pod is ownerless (set in correlation.newIncident).
	//   - workload-object incidents (deployment, statefulset, job, ingress,
	//     service, networkpolicy, ...): fully-qualified "namespace/name".
	//   - node incidents: the node name.
	//   - smart-group incidents: a human-readable member summary.
	Name                   string
	Count                  int
	FirstSeen              time.Time
	LastSeen               time.Time
	Resources              map[string]bool
	PeakResources          int
	Containers             map[string]bool
	OwnerKind              string
	ContainerName          string
	Image                  string
	RestartCount           int
	Hint                   string
	Runbook                string
	Logs                   string
	Events                 string
	State                  IncidentState
	LastUpdate             time.Time
	LastContainerState     *ContainerState
	Severity               Severity
	SuppressedPods         int
	SuppressedOwners       map[string]int // owner → count of suppressed pods
	SuppressedPodSummaries []PodSummary
	ResolveAt              time.Time
	IncludeEvents          bool
	IncludeLogs            bool
	NodeName               string
	NotifiedSig            string
	LastNotifiedAt         time.Time
	RenotifyCount          int
}

// PersistedIncident is a lightweight serializable subset of Incident,
// stored in the kwatch-incidents ConfigMap to survive restarts.
type PersistedIncident struct {
	Key            IncidentKey     `json:"key"`
	Reason         string          `json:"reason"`
	Namespace      string          `json:"namespace"`
	Name           string          `json:"name"`
	Resource       string          `json:"resource"`
	Count          int             `json:"count"`
	FirstSeen      time.Time       `json:"firstSeen"`
	LastSeen       time.Time       `json:"lastSeen"`
	Resources      map[string]bool `json:"resources"`
	PeakResources  int             `json:"peakResources"`
	OwnerKind      string          `json:"ownerKind"`
	RestartCount   int             `json:"restartCount"`
	Hint           string          `json:"hint"`
	Severity       Severity        `json:"severity"`
	State          IncidentState   `json:"state"`
	ResolveAt      time.Time       `json:"resolveAt,omitempty"`
	NotifiedSig    string          `json:"notifiedSig"`
	LastNotifiedAt time.Time       `json:"lastNotifiedAt"`
	RenotifyCount  int             `json:"renotifyCount"`
}

// ToPersisted converts an Incident into its serializable subset.
func (inc *Incident) ToPersisted() PersistedIncident {
	return PersistedIncident{
		Key:            inc.Key,
		Reason:         inc.Reason,
		Namespace:      inc.Namespace,
		Name:           inc.Name,
		Resource:       inc.Resource,
		Count:          inc.Count,
		FirstSeen:      inc.FirstSeen,
		LastSeen:       inc.LastSeen,
		Resources:      inc.Resources,
		PeakResources:  inc.PeakResources,
		OwnerKind:      inc.OwnerKind,
		RestartCount:   inc.RestartCount,
		Hint:           inc.Hint,
		Severity:       inc.Severity,
		State:          inc.State,
		ResolveAt:      inc.ResolveAt,
		NotifiedSig:    inc.NotifiedSig,
		LastNotifiedAt: inc.LastNotifiedAt,
		RenotifyCount:  inc.RenotifyCount,
	}
}

// ToIncident converts a PersistedIncident back to a full Incident.
func (pi *PersistedIncident) ToIncident() *Incident {
	return &Incident{
		Key:            pi.Key,
		Reason:         pi.Reason,
		Namespace:      pi.Namespace,
		Name:           pi.Name,
		Resource:       pi.Resource,
		Count:          pi.Count,
		FirstSeen:      pi.FirstSeen,
		LastSeen:       pi.LastSeen,
		Resources:      pi.Resources,
		PeakResources:  pi.PeakResources,
		OwnerKind:      pi.OwnerKind,
		RestartCount:   pi.RestartCount,
		Hint:           pi.Hint,
		Severity:       pi.Severity,
		State:          pi.State,
		NotifiedSig:    pi.NotifiedSig,
		LastNotifiedAt: pi.LastNotifiedAt,
		RenotifyCount:  pi.RenotifyCount,
		ResolveAt:      pi.ResolveAt,
		Containers:     make(map[string]bool),
		LastUpdate:     pi.LastSeen,
	}
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
	if len(inc.SuppressedPodSummaries) > 0 {
		c.SuppressedPodSummaries = make([]PodSummary, len(inc.SuppressedPodSummaries))
		copy(c.SuppressedPodSummaries, inc.SuppressedPodSummaries)
	}
	return &c
}
