package model

import "time"

type IncidentAction int

const (
	ActionCreate IncidentAction = iota
	ActionUpdate
	ActionSkip
	ActionStale
	ActionResolved
)

type IncidentState int

const (
	StateActive IncidentState = iota
	StateStale
	StateResolved
)

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
}
