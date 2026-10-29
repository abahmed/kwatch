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

// Evidence caps, the message's own: four facts and five quoted lines.
const (
	MaxEvidenceFacts = 4
	MaxQuoteLines    = 5
)
