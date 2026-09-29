package health

import "time"

// MaxProblemViews bounds the /problems response.
const MaxProblemViews = 200

// ProblemView is the safe diagnostic summary of one problem. It carries no
// raw evidence, logs, or object data.
type ProblemView struct {
	ID          string    `json:"id"`
	State       string    `json:"state"`
	Tier        string    `json:"tier"`
	Root        RootView  `json:"root"`
	Cause       string    `json:"cause,omitempty"`
	Confidence  float64   `json:"confidence,omitempty"`
	Opened      time.Time `json:"opened,omitzero"`
	Announced   time.Time `json:"announced,omitzero"`
	Members     int       `json:"members"`
	ImpactCount int       `json:"impactCount"`
}

// RootView names the entity a problem is rooted in.
type RootView struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}
