package incident

import "time"

// Section statuses. A check that read what it needed is ok; one the API
// refused is denied, one that ran out of time is timeout, and one whose
// object is not there is absent. A denied read never fails the
// investigation: the other sections still show.
const (
	SectionOK      = "ok"
	SectionDenied  = "denied"
	SectionTimeout = "timeout"
	SectionAbsent  = "absent"
)

// Section reasons say why a section is denied.
const (
	// ReasonRBAC: the API refused the read.
	ReasonRBAC = "rbac"
	// ReasonOff: the read is turned off in configuration
	// (investigation.logs: false).
	ReasonOff = "off"
)

// Check names. Each names one read-only look a suggested step would
// otherwise ask the reader to take; compose drops a read-only step in
// Pro when the check that covers it came back ok.
const (
	CheckPodState     = "pod-state"
	CheckLogs         = "logs"
	CheckEvents       = "events"
	CheckUsage        = "usage"
	CheckNode         = "node"
	CheckNodePods     = "node-pods"
	CheckVolume       = "volume"
	CheckConfig       = "config"
	CheckService      = "service"
	CheckIngress      = "ingress"
	CheckAdmission    = "admission"
	CheckRollout      = "rollout"
	CheckControlPlane = "control-plane"
	CheckFinalizer    = "finalizer"
	CheckObject       = "object"
	CheckRegistry     = "registry"
	CheckScheduling   = "scheduling"
	CheckCrash        = "crash"
	CheckInitWait     = "init-wait"
	CheckImage        = "image"
	CheckCertificate  = "certificate"
	CheckQuota        = "quota"
	CheckHPA          = "hpa"
	CheckJob          = "job"
	CheckProbe        = "probe"
)

// StepChecks are the checks that cover a read-only suggested step. The
// investigate package registers a check for every one of them, which a
// test there enforces.
var StepChecks = []string{
	CheckPodState, CheckLogs, CheckUsage, CheckNode, CheckNodePods,
	CheckVolume, CheckConfig, CheckIngress, CheckAdmission, CheckRollout,
	CheckControlPlane, CheckFinalizer, CheckObject,
}

// SectionFact is one labelled value of a section, such as "Phase" and
// "Running". Values are redacted; Secret and ConfigMap values are never
// read, only key names.
type SectionFact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Section is everything one check read, for the dashboard. Its size is
// bounded by investigate's report caps, not the message caps.
type Section struct {
	Check  string `json:"check"`
	Title  string `json:"title"`
	Status string `json:"status"`
	// Reason says why a denied section is denied (ReasonRBAC, ReasonOff).
	Reason string `json:"reason,omitempty"`
	// Subject names what the section is about, such as a container.
	Subject string `json:"subject,omitempty"`
	// Run is "previous" or "current" for a log section.
	Run   string        `json:"run,omitempty"`
	Facts []SectionFact `json:"facts,omitempty"`
	Lines []string      `json:"lines,omitempty"`
	// Matched indexes the Lines that carry the failure's signature.
	Matched []int `json:"matched,omitempty"`
	// At is when the check returned, from the investigator's clock.
	At time.Time `json:"at,omitempty"`
}

// Correlation is what kwatch concluded across sections: the classified
// log signature and the object it points to, the nearest prior change,
// the spread and, for a rollout, what differs from the last healthy
// revision. Confirmed is set when gathered evidence proves the cause.
type Correlation struct {
	Facts []SectionFact `json:"facts,omitempty"`
	// Confirmed says why the cause is confirmed: "orders-db has 0 ready
	// endpoints". Empty when the evidence did not prove it.
	Confirmed string `json:"confirmed,omitempty"`
}

// Correlation fact labels, in the order the dashboard shows them.
const (
	CorrelationSignature = "Signature"
	CorrelationMatches   = "Matches"
	CorrelationChange    = "Change"
	CorrelationSpread    = "Spread"
	CorrelationDiff      = "Before and after"
)

// GatheredChecks are the checks of report whose read came back ok.
func GatheredChecks(report []Section) map[string]bool {
	out := map[string]bool{}
	for _, s := range report {
		if s.Status == SectionOK {
			out[s.Check] = true
		}
	}
	return out
}
