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

// Facts are the structured details behind an incident's hint.
//
// The hint is prose for humans. Renderers used to parse it back with string
// searches and regexes to recover the memory limit, the probe endpoint or the
// scheduling delay, so every rewording of a hint silently broke a rendered
// section. Producers fill these in at the moment they build the hint;
// renderers read them and never look inside the hint again.
type Facts struct {
	// MemoryLimit is the container's memory limit when it was OOM-killed,
	// e.g. "256Mi". Empty when no limit was set.
	MemoryLimit string `json:"memoryLimit,omitempty"`
	// OOMTimeline lists the kills seen inside the OOM window, as the tracker
	// renders them, e.g. "[12:01, 12:04, 12:09]".
	OOMTimeline string `json:"oomTimeline,omitempty"`
	// OOMCount and OOMWindowMin are the kill count and window behind a
	// repeating-OOM (memory leak) signal.
	OOMCount     int  `json:"oomCount,omitempty"`
	OOMWindowMin int  `json:"oomWindowMin,omitempty"`
	MemoryLeak   bool `json:"memoryLeak,omitempty"`
	// ProbeEndpoint is what the failing probe checks, e.g.
	// "HTTP GET http://app:8080/healthz", "TCP check :5432", "exec /ready.sh".
	ProbeEndpoint string `json:"probeEndpoint,omitempty"`
	// PullSecretsSet records that the pod declares imagePullSecrets, which
	// turns an image-pull failure from "add credentials" into "check them".
	PullSecretsSet bool `json:"pullSecretsSet,omitempty"`
	// SchedulingDelay is how long a pod has been unschedulable.
	SchedulingDelay time.Duration `json:"schedulingDelay,omitempty"`
	// ResourceRequests summarises each container's requests for an
	// unschedulable pod, e.g. "app requests: cpu=500m mem=1Gi".
	ResourceRequests []string `json:"resourceRequests,omitempty"`
}

// IsZero reports whether no fact is set.
func (f Facts) IsZero() bool {
	return f.MemoryLimit == "" && f.OOMTimeline == "" && f.OOMCount == 0 &&
		f.OOMWindowMin == 0 &&
		!f.MemoryLeak &&
		f.ProbeEndpoint == "" &&
		!f.PullSecretsSet &&
		f.SchedulingDelay == 0 &&
		len(f.ResourceRequests) == 0
}

// clone returns a copy that shares no memory with f.
func (f Facts) clone() Facts {
	c := f
	if f.ResourceRequests != nil {
		c.ResourceRequests = append([]string(nil), f.ResourceRequests...)
	}
	return c
}

// Incident is one problem kwatch is tracking. It is written by several
// subsystems, so it is composed of five parts, each with one owner:
//
//   - Subject: what the incident is about. Set by correlation.newIncident;
//     the enricher may refresh Image and NodeName as replicas come and go.
//   - Status: what is happening now. Refreshed by the correlation engine on
//     every matching event.
//   - Evidence: why, and what we saw. Filled by the enricher from the event.
//   - Attribution: how it relates to other incidents. Filled by the
//     attribution stage.
//   - Delivery: notification bookkeeping. Written only by the engine's edge
//     detection and renotify logic.
//
// The parts are embedded, so inc.Reason and inc.Count read as before; only
// composite literals name the part. Renderers read Incident and never write
// it; the alert manager receives a clone.
type Incident struct {
	Subject
	Status
	Evidence
	Attribution
	Delivery
}

// Subject identifies what an incident is about.
type Subject struct {
	ID        string // stable short hash for log correlation
	Key       IncidentKey
	Reason    string
	Namespace string
	Resource  string
	// Name identifies the subject of the incident; its encoding depends on the
	// resource kind, so compare via ParseKey/OwnerPath rather than raw
	// equality:
	//   - pod incidents: bare owning-workload name, or the pod's own name when
	//     the pod is ownerless (set in correlation.newIncident).
	//   - workload-object incidents (deployment, statefulset, job, ingress,
	//     service, networkpolicy, ...): fully-qualified "namespace/name".
	//   - node incidents: the node name.
	//   - smart-group incidents: a human-readable member summary.
	Name          string
	OwnerKind     string
	ContainerName string
	Image         string
	NodeName      string
}

// Status is the live state of an incident.
type Status struct {
	State         IncidentState
	Severity      Severity
	Count         int
	FirstSeen     time.Time
	LastSeen      time.Time
	LastUpdate    time.Time
	ResolveAt     time.Time
	Resources     map[string]bool
	PeakResources int
	Containers    map[string]bool
	RestartCount  int
	// LastContainerState is the most recent container status seen for this
	// incident; renderers show its message and exit code.
	LastContainerState *ContainerState
}

// Evidence is the explanation attached to an incident and the material it
// rests on.
type Evidence struct {
	Hint string
	// Facts are the structured details behind Hint; see Facts.
	Facts   Facts
	Runbook string
	Logs    string
	Events  string
	// EvidencePod names the pod the attached Logs and Events were collected
	// from. Incidents are keyed by owner, not by pod, so one incident outlives
	// the replicas it describes — without this the alert can list one pod under
	// Resources while showing another pod's events.
	EvidencePod   string
	IncludeEvents bool
	IncludeLogs   bool
	// AffectedServices are the Services whose selectors match the failing
	// pods, resolved from live Service objects. They are impact, not
	// explanation, so they live here rather than in Hint.
	AffectedServices []string
	// OwnerUnhealthy records that the owning workload is itself unhealthy —
	// the pod failure is a symptom of a rollout, not an isolated crash.
	OwnerUnhealthy bool
}

// Attribution is what the attribution stage recorded about this incident's
// relationship to other incidents: the symptoms it speaks for, or the cause
// that speaks for it. See correlation/attribution.go.
type Attribution struct {
	// SuppressedBy is the mass-failure incident that is speaking for this one.
	// A suppressed incident is tracked — it counts toward the mass failure and
	// resolves normally — but is not announced. When the mass failure clears
	// while this incident is still active, it is released and announced then,
	// so a symptom that outlives its cause is not lost.
	SuppressedBy IncidentKey
	// SuppressedPods counts the pod symptoms a node incident speaks for;
	// SuppressedOwners breaks them down by owning workload and
	// SuppressedPodSummaries keeps the first few for display.
	SuppressedPods         int
	SuppressedOwners       map[string]int // owner → count of suppressed pods
	SuppressedPodSummaries []PodSummary
}

// Delivery is the correlation engine's notification bookkeeping. Only the
// engine writes it: NotifiedSig is the last announced state (edge detection
// compares against it), LastNotifiedAt and RenotifyCount drive the renotify
// budget.
type Delivery struct {
	NotifiedSig    string
	LastNotifiedAt time.Time
	RenotifyCount  int
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
	Facts          Facts           `json:"facts,omitempty"`
	Severity       Severity        `json:"severity"`
	State          IncidentState   `json:"state"`
	ResolveAt      time.Time       `json:"resolveAt,omitempty"`
	NotifiedSig    string          `json:"notifiedSig"`
	LastNotifiedAt time.Time       `json:"lastNotifiedAt"`
	RenotifyCount  int             `json:"renotifyCount"`
	SuppressedBy   IncidentKey     `json:"suppressedBy,omitempty"`
}

// ToPersisted converts an Incident into its serializable subset.
func (inc *Incident) ToPersisted() PersistedIncident {
	resources := make(map[string]bool, len(inc.Resources))
	for k, v := range inc.Resources {
		resources[k] = v
	}
	return PersistedIncident{
		Key:            inc.Key,
		Reason:         inc.Reason,
		Namespace:      inc.Namespace,
		Name:           inc.Name,
		Resource:       inc.Resource,
		Count:          inc.Count,
		FirstSeen:      inc.FirstSeen,
		LastSeen:       inc.LastSeen,
		Resources:      resources,
		PeakResources:  inc.PeakResources,
		OwnerKind:      inc.OwnerKind,
		RestartCount:   inc.RestartCount,
		Hint:           inc.Hint,
		Facts:          inc.Facts.clone(),
		Severity:       inc.Severity,
		State:          inc.State,
		ResolveAt:      inc.ResolveAt,
		NotifiedSig:    inc.NotifiedSig,
		LastNotifiedAt: inc.LastNotifiedAt,
		RenotifyCount:  inc.RenotifyCount,
		SuppressedBy:   inc.SuppressedBy,
	}
}

// ToIncident converts a PersistedIncident back to a full Incident.
func (pi *PersistedIncident) ToIncident() *Incident {
	// Every refresh path writes into Resources; a nil map from an older or
	// hand-edited ConfigMap would panic the engine on the first event.
	resources := pi.Resources
	if resources == nil {
		resources = make(map[string]bool)
	}
	return &Incident{
		Subject: Subject{
			Key:       pi.Key,
			Reason:    pi.Reason,
			Namespace: pi.Namespace,
			Name:      pi.Name,
			Resource:  pi.Resource,
			OwnerKind: pi.OwnerKind,
		},
		Status: Status{
			Count:         pi.Count,
			FirstSeen:     pi.FirstSeen,
			LastSeen:      pi.LastSeen,
			Resources:     resources,
			PeakResources: pi.PeakResources,
			RestartCount:  pi.RestartCount,
			Severity:      pi.Severity,
			State:         pi.State,
			ResolveAt:     pi.ResolveAt,
			Containers:    make(map[string]bool),
			LastUpdate:    pi.LastSeen,
		},
		Evidence: Evidence{
			Hint:  pi.Hint,
			Facts: pi.Facts.clone(),
		},
		Attribution: Attribution{
			SuppressedBy: pi.SuppressedBy,
		},
		Delivery: Delivery{
			NotifiedSig:    pi.NotifiedSig,
			LastNotifiedAt: pi.LastNotifiedAt,
			RenotifyCount:  pi.RenotifyCount,
		},
	}
}

// Clone returns a deep copy of the incident, safe for concurrent use.
func (inc *Incident) Clone() *Incident {
	c := *inc
	c.Facts = inc.Facts.clone()
	if inc.AffectedServices != nil {
		c.AffectedServices = append([]string(nil), inc.AffectedServices...)
	}
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
		c.SuppressedPodSummaries = make(
			[]PodSummary,
			len(inc.SuppressedPodSummaries),
		)
		copy(c.SuppressedPodSummaries, inc.SuppressedPodSummaries)
	}
	return &c
}
