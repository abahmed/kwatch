package notification

// Evidence is what Pro gathered for a message, for providers that lay it
// out themselves (Teams, email, the generic webhook). Compose sets it only
// for a Pro message whose incident was investigated, so every other
// message and payload stays as it was.
type Evidence struct {
	// Confirmed is set when gathered evidence proves the cause.
	Confirmed bool `json:"confirmed"`
	// Checks counts the checks that ran and read an answer.
	Checks int `json:"checks"`
	// Facts are the key facts kwatch read, most decisive first.
	Facts []string `json:"facts,omitempty"`
	// Quote is the log tail that carries the failure's signature.
	Quote *Quote `json:"quote,omitempty"`
	// Fix lists the steps that change the cluster; every one is
	// Mutating.
	Fix []EvidenceStep `json:"fix,omitempty"`
	// Checked says, for an issue without a sure cause, what kwatch
	// checked and found fine and what it could not check, one plain
	// sentence each: "kwatch checked: its node is healthy; …", "kwatch
	// cannot read pod logs here." Empty when nothing was checked.
	Checked []string `json:"checked,omitempty"`
	// Grouped says, for an issue that groups several workloads or
	// objects, what ties them together: "Grouped because all 12 pods
	// run on node n1, which is not ready." Empty otherwise.
	Grouped string `json:"grouped,omitempty"`
	// Replicas say, when only some replicas of one workload fail, what
	// the failing ones share that the healthy ones do not, at most two
	// sentences: "Only the 3 pods on arm64 nodes fail; the 4 pods on
	// amd64 nodes are healthy." An observation, never the cause.
	Replicas []string `json:"replicas,omitempty"`
	// Users says whether users reach what fails and how much is left
	// to serve them: "Users are still served: route shop.example.com
	// → api has 2 of 3 ready endpoints." Empty when kwatch cannot say.
	Users string `json:"users,omitempty"`
}

// Quote is a few log lines of one container run.
type Quote struct {
	Container string   `json:"container"`
	Run       string   `json:"run"`
	Lines     []string `json:"lines"`
}

// EvidenceStep is one fix step as the webhook's evidence object names it.
type EvidenceStep struct {
	Text     string `json:"text"`
	Command  string `json:"command,omitempty"`
	Mutating bool   `json:"mutating"`
}

// Evidence caps, the message's own: four facts, five quoted lines and
// five checked items.
const (
	MaxEvidenceFacts   = 4
	MaxQuoteLines      = 5
	MaxEvidenceChecked = 5
	MaxEvidenceReplica = 2
)

// Notes are the short lines a card or mail shows after the facts: the
// users line, what groups the issue and at most MaxEvidenceReplica
// replica observations. Unset fields add nothing.
func (e *Evidence) Notes() []string {
	var out []string
	for _, s := range []string{e.Users, e.Grouped} {
		if s != "" {
			out = append(out, s)
		}
	}
	return append(out, capped(e.Replicas, MaxEvidenceReplica)...)
}

// CheckedShown is Checked cut to MaxEvidenceChecked items.
func (e *Evidence) CheckedShown() []string {
	return capped(e.Checked, MaxEvidenceChecked)
}

func capped(items []string, limit int) []string {
	if len(items) > limit {
		return items[:limit]
	}
	return items
}
