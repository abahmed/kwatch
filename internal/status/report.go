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
	// CauseChecks says, per propagation rule, how often recovery
	// followed a change to the named cause. It is display only.
	CauseChecks []CauseCheck `json:"causeChecks,omitempty"`
	// Unexplained counts, over the last seven days, what kwatch saw but
	// could not explain. It is display only.
	Unexplained Unexplained `json:"unexplained"`
	// Edition names the product edition ("Community" or "Pro"); empty
	// for a report that does not say.
	Edition string `json:"edition,omitempty"`
	// ProInactive names the Pro features the configuration sets and the
	// edition does not run.
	ProInactive []string `json:"proFeaturesInactive,omitempty"`
	// ProDetectorsOff names the Pro detector families the edition does
	// not run.
	ProDetectorsOff []string `json:"proDetectorsNotRunning,omitempty"`
	// CommunitySends names the provider a capped edition sends to and the
	// ones it leaves inactive, such as "slack (inactive: pagerduty)".
	CommunitySends string `json:"communitySends,omitempty"`
	// License is the licensing state; nil for a report that does not say.
	License *License `json:"license,omitempty"`
	// Brief leaves the self-check and unexplained sections out of the
	// text. Community reports are brief.
	Brief bool `json:"-"`
}

// Unexplained is the unexplained counts of the last seven days.
type Unexplained struct {
	// Events counts Warning events whose reason kwatch has no entry
	// for; Incidents counts incidents whose cause stayed unclear.
	Events    int `json:"events"`
	Incidents int `json:"incidents"`
	// Top are the largest reasons, most first.
	Top []UnexplainedRow `json:"top,omitempty"`
}

// UnexplainedRow is one failure mode and quoted reason with its count.
type UnexplainedRow struct {
	// Kind is "event" or "incident".
	Kind   string `json:"kind"`
	Mode   string `json:"mode"`
	Reason string `json:"reason"`
	Count  int    `json:"count"`
	// Sample is the latest quoted evidence, already redacted.
	Sample string `json:"sample,omitempty"`
}

// CauseCheck is one rule's self-check over the last seven days.
type CauseCheck struct {
	Rule           string `json:"rule"`
	Confirmed      int    `json:"confirmed"`
	OtherChanged   int    `json:"otherChanged"`
	NothingChanged int    `json:"nothingChanged"`
	Total          int    `json:"total"`
	// Text is "cause confirmed by the fix in N of M resolved incidents
	// (last 7 days)".
	Text string `json:"text"`
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

// ConflictHint tells an operator whose license is bound to another cluster
// where to move it.
const ConflictHint = "license is bound to another cluster — move it " +
	"from the installer's License / Pro menu or transfer it at " +
	"https://kwatch.dev/account"

// License is the licensing section of /status. It never carries the key,
// only the masked license ID.
type License struct {
	Edition string `json:"edition"`
	State   string `json:"state"`
	Plan    string `json:"plan,omitempty"`
	// LID is the masked license ID.
	LID string `json:"lid,omitempty"`
	// DaysLeft counts whole days to the license expiry, never negative.
	DaysLeft    *int       `json:"daysLeft,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	GraceUntil  *time.Time `json:"graceUntil,omitempty"`
	LastRenew   *time.Time `json:"lastRenew,omitempty"`
	NextAttempt *time.Time `json:"nextAttempt,omitempty"`
	Reason      string     `json:"reason,omitempty"`
	Hint        string     `json:"hint,omitempty"`
}
