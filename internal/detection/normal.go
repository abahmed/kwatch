package detection

// Normality is how a finding compares with the behaviour its workload
// has shown over the last week. It looks backwards only: it says what
// the workload usually did, never what it will do.
type Normality uint8

// Normality values.
const (
	// NormalUnjudged is a finding that was not compared, because the
	// workload has too little history or the finding is not of a kind
	// that history can excuse.
	NormalUnjudged Normality = iota
	// NormalUsual is behaviour inside the workload's normal range, such
	// as a batch worker that always restarts twice an hour.
	NormalUsual
	// NormalUnusual is behaviour well beyond the workload's normal
	// range, such as restarts from a workload that never restarts.
	NormalUnusual
)

// EvidenceBaseline is the finding's figure beside the workload's usual
// one, written as a phrase: "restarts 12×/h vs a usual 0.1×/h". The
// message writer quotes it.
const EvidenceBaseline = "baseline"
