package status

import "time"

// Bounds keep /status small however big the cluster is.
const (
	MaxProblems = 20
	MaxZones    = 12
	MaxGaps     = 25
	maxLine     = 240
)

// Report is everything /status shows.
type Report struct {
	At      time.Time `json:"at"`
	Cluster string    `json:"cluster,omitempty"`
	// Problems are the open incidents.
	Problems Problems `json:"problems"`
	// ControlPlane is the state of the components kwatch probes.
	ControlPlane []Component `json:"controlPlane"`
	// Upgrade answers "is it safe to upgrade the cluster?".
	Upgrade Readiness `json:"upgradeReadiness"`
	// Zones is the state of each availability zone.
	Zones Zones `json:"zones"`
	// Gaps is what kwatch cannot see.
	Gaps []Gap `json:"coverageGaps"`
}

// Problems are the open incidents, the worst first.
type Problems struct {
	Open  int       `json:"open"`
	Items []Problem `json:"items"`
	// More counts the open incidents the list leaves out.
	More int `json:"more,omitempty"`
}

// Problem is one open incident.
type Problem struct {
	ID    string `json:"id"`
	Tier  string `json:"tier"`
	State string `json:"state"`
	// Root is the entity blamed, as "kind namespace/name".
	Root       string    `json:"root"`
	OpenedAt   time.Time `json:"openedAt"`
	AgeSeconds int64     `json:"ageSeconds"`
	// Cause is the one-line statement a message would lead with.
	Cause string `json:"cause"`
}

// Component is one control plane component.
type Component struct {
	Name string `json:"name"`
	// State is "ok", "problem" or "unknown" (not probed).
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// Zones is the zone health, assessed only with two or more zones.
type Zones struct {
	Assessed bool   `json:"assessed"`
	Items    []Zone `json:"items,omitempty"`
}

// Zone is the state of one availability zone.
type Zone struct {
	Name        string `json:"name"`
	Nodes       int    `json:"nodes"`
	NotReady    int    `json:"nodesNotReady"`
	FailingPods int    `json:"failingPods"`
	// Concentrated is set when every failure of the cluster is in this
	// zone and the other zones are healthy.
	Concentrated bool `json:"concentrated,omitempty"`
}

// Gap is something kwatch cannot see, and why.
type Gap struct {
	What   string `json:"what"`
	Reason string `json:"reason"`
}
