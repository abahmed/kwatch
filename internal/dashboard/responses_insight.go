package dashboard

import "time"

// RecentWarning is one Kubernetes Warning event about the incident's
// objects, the same reason and message folded into one: Count adds
// their counts, LastSeen is the newest, Object names the object of the
// newest and Objects how many objects reported it. Message is redacted.
type RecentWarning struct {
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Count     int       `json:"count"`
	FirstSeen time.Time `json:"firstSeen,omitzero"`
	LastSeen  time.Time `json:"lastSeen"`
	Object    string    `json:"object"`
	Objects   int       `json:"objects"`
}

// TrafficRow is one Service or Ingress the incident affects and how
// many of its endpoints are ready now. An Ingress counts the endpoints
// of the Services it routes to, named in Backends.
type TrafficRow struct {
	Kind      string   `json:"kind"`
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Ready     int      `json:"ready"`
	Total     int      `json:"total"`
	Backends  []string `json:"backends,omitempty"`
	// Reach is where users reach it: up to maxTraffic host/path rows,
	// the ones without ready endpoints first (detail_reach.go).
	Reach []ReachRow `json:"reach,omitempty"`
}

// NodeRow is one node the incident involves: the root node, or a node
// the incident's pods run on. CapacityType is spot or on-demand, empty
// when no well-known label says. Conditions are the node conditions
// that are not healthy now. Pods counts the incident's pods on it and
// OtherFailing the other pods on it that are not running well.
type NodeRow struct {
	Name         string          `json:"name"`
	Root         bool            `json:"root,omitempty"`
	Zone         string          `json:"zone,omitempty"`
	Pool         string          `json:"pool,omitempty"`
	InstanceType string          `json:"instanceType,omitempty"`
	CapacityType string          `json:"capacityType,omitempty"`
	Created      time.Time       `json:"created,omitzero"`
	Ready        *bool           `json:"ready,omitempty"`
	Cordoned     bool            `json:"cordoned,omitempty"`
	Deleting     bool            `json:"deleting,omitempty"`
	Conditions   []NodeCondition `json:"conditions,omitempty"`
	Pods         int             `json:"pods"`
	OtherFailing int             `json:"otherFailing"`
}

// NodeCondition is one node condition that is not healthy. Message is
// redacted.
type NodeCondition struct {
	Type    string    `json:"type"`
	Status  string    `json:"status"`
	Reason  string    `json:"reason,omitempty"`
	Message string    `json:"message,omitempty"`
	Since   time.Time `json:"since,omitzero"`
}

// ProbeRow is the probe of one container that keeps failing: what it
// checks ("http /healthz :8080", "tcp :5432", "exec sh"; never an exec
// command's arguments), how it is timed in seconds, and the latest
// failure the kubelet reported (redacted), with how often it repeated.
type ProbeRow struct {
	Namespace   string    `json:"namespace"`
	Pod         string    `json:"pod"`
	Container   string    `json:"container"`
	Probe       string    `json:"probe"`
	Check       string    `json:"check,omitempty"`
	Delay       int       `json:"delaySeconds"`
	Timeout     int       `json:"timeoutSeconds,omitempty"`
	Period      int       `json:"periodSeconds,omitempty"`
	Failures    int       `json:"failureThreshold,omitempty"`
	LastMessage string    `json:"lastMessage,omitempty"`
	LastAt      time.Time `json:"lastAt,omitzero"`
	Count       int       `json:"count,omitempty"`
}

// Waiting is why the incident's pods are waiting to be placed on a
// node: the scheduler's latest message (redacted) about Pod and, when
// kwatch recognizes them, its reasons in plain words with how many of
// Nodes nodes each rules out. Pods counts the pods still waiting.
type Waiting struct {
	Pods      int          `json:"pods"`
	Namespace string       `json:"namespace"`
	Pod       string       `json:"pod"`
	Message   string       `json:"message"`
	At        time.Time    `json:"at,omitzero"`
	Nodes     int          `json:"nodes,omitempty"`
	Reasons   []WaitReason `json:"reasons,omitempty"`
}

// WaitReason is one reason the scheduler ruled nodes out: Text in
// plain words, Reason the scheduler's own words.
type WaitReason struct {
	Text   string `json:"text"`
	Reason string `json:"reason"`
	Nodes  int    `json:"nodes"`
}
