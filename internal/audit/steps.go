package audit

// Step kinds of a StepRecord. The strings are stable: people grep audit
// logs for them.
const (
	// StepReadOnly is a command that changes nothing.
	StepReadOnly = "read-only"
	// StepMutating is a command that changes the cluster.
	StepMutating = "changes the cluster"
	// StepAdvice is a step with no command, such as a revert in Git.
	StepAdvice = "advice"
)

// Bounds of the steps an entry carries, so a line stays small.
const (
	MaxSteps    = 8
	MaxStepText = 300
)

// StepRecord is one step a message suggested. Command is the command as
// shown, Text the step's words when it has no command; both are
// redacted before they reach the log, and cut at MaxStepText characters.
type StepRecord struct {
	Kind    string `json:"kind"`
	Command string `json:"command,omitempty"`
	Text    string `json:"text,omitempty"`
}
