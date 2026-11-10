package release

import (
	"sort"
	"time"
)

// Verdict is how a release went.
type Verdict string

const (
	// Rolling: the new pods are still replacing the old ones.
	Rolling Verdict = "rolling"
	// Watching: rolled out, still inside the watch window.
	Watching Verdict = "watching"
	// Healthy: rolled out and nothing broke inside the watch window.
	Healthy Verdict = "healthy"
	// Regressed: the new version restarts far more than the previous.
	Regressed Verdict = "regressed"
	// Failed: the new version fails while the workload still serves.
	Failed Verdict = "failed"
	// Down: the workload has no ready replica.
	Down Verdict = "down"
	// Replaced: a newer release came before a verdict.
	Replaced Verdict = "replaced"
	// Unknown: kwatch saw the release only after the fact, so it could
	// not watch it roll out.
	Unknown Verdict = "unknown"
)

// severity orders the bad verdicts: a release keeps the worst it reached
// inside its watch window.
func (v Verdict) severity() int {
	switch v {
	case Down:
		return 3
	case Failed:
		return 2
	case Regressed:
		return 1
	}
	return 0
}

// Bad reports a verdict that says the release went wrong.
func (v Verdict) Bad() bool { return v.severity() > 0 }

// Image is one container's image before and after the release. Before
// is empty for a container the release added.
type Image struct {
	Container string `json:"container"`
	Before    string `json:"before,omitempty"`
	After     string `json:"after"`
}

// Record is one release of one workload.
type Record struct {
	// ID names the release: kind/namespace/name@unix-seconds, with
	// "-n" for the nth release of the workload within that second.
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Namespace string    `json:"namespace"`
	Name      string    `json:"name"`
	At        time.Time `json:"at"`
	// Images are the containers whose image changed.
	Images []Image `json:"images,omitempty"`
	// Changed names what else the release changed: "environment",
	// "resources", "probes", "config", "volumes", "restart" or
	// "pod template".
	Changed []string `json:"changed,omitempty"`
	// Actor, App and Cause say who and how, when the change journal
	// knows: the field manager, the GitOps application and the
	// change-cause annotation.
	Actor string `json:"actor,omitempty"`
	App   string `json:"app,omitempty"`
	Cause string `json:"cause,omitempty"`
	// Revision is the workload revision to go back to, when known.
	Revision string  `json:"revision,omitempty"`
	Verdict  Verdict `json:"verdict"`
	// Final is set once the verdict no longer changes.
	Final bool `json:"final,omitempty"`
	// RolledOut is when every replica ran the new version.
	RolledOut time.Time `json:"rolledOut,omitzero"`
	// Stalled says the rollout stopped: the Deployment's progress
	// deadline passed, or the rollout did not finish in StallAfter.
	Stalled bool `json:"stalled,omitempty"`
	// Deadline says the stall was the Deployment's progress deadline.
	Deadline bool `json:"deadline,omitempty"`
	// Progress is the updated plus ready replicas last seen, and
	// ProgressAt when it last changed: a rollout without a progress
	// deadline stalls when it stops moving, not when it is slow.
	Progress   int       `json:"progress,omitempty"`
	ProgressAt time.Time `json:"progressAt,omitzero"`
	// Ready and Desired are the replicas last seen; OldServing says
	// ready replicas still ran the previous version.
	Ready      int  `json:"ready"`
	Desired    int  `json:"desired"`
	OldServing bool `json:"oldServing,omitempty"`
	// Incident is the issue the release caused, when one did.
	Incident string `json:"incident,omitempty"`
	// Detail is the health comparison and the error line (Pro).
	Detail *Detail `json:"detail,omitempty"`
}

// Detail compares the workload before and after the release.
type Detail struct {
	// RestartsBefore is the workload's usual restarts per hour, from
	// its baseline; RestartsAfter the new pods' rate since the release.
	RestartsBefore *float64 `json:"restartsBefore,omitempty"`
	RestartsAfter  *float64 `json:"restartsAfter,omitempty"`
	// ReadyBefore is the workload's usual seconds to ready.
	ReadyBefore *float64 `json:"readyBefore,omitempty"`
	// ErrorLine is the first fatal line a new container stopped with.
	ErrorLine string `json:"errorLine,omitempty"`
}

// Limits of the book.
const (
	// MaxRecords is the most releases kept.
	MaxRecords = 500
	// KeepFor is how long a release is kept.
	KeepFor = 7 * 24 * time.Hour
)

// Book is the releases kept, newest first.
type Book struct {
	Records []Record `json:"records"`
}

// find returns the record with id, or nil.
func (b *Book) find(id string) *Record {
	for i := range b.Records {
		if b.Records[i].ID == id {
			return &b.Records[i]
		}
	}
	return nil
}

// add keeps r, newest first, within MaxRecords.
func (b *Book) add(r Record) {
	b.Records = append(b.Records, r)
	sort.SliceStable(b.Records, func(i, j int) bool {
		return b.Records[i].At.After(b.Records[j].At)
	})
	if len(b.Records) > MaxRecords {
		b.Records = b.Records[:MaxRecords]
	}
}

// tooOld reports a release a full book would drop at once: older than
// every one it keeps. Adding it would only truncate it again on every
// look, and report a change each time.
func (b *Book) tooOld(at time.Time) bool {
	return len(b.Records) >= MaxRecords &&
		!at.After(b.Records[len(b.Records)-1].At)
}

// prune drops the releases older than KeepFor; true when one went.
func (b *Book) prune(now time.Time) bool {
	kept := b.Records[:0]
	for _, r := range b.Records {
		if now.Sub(r.At) <= KeepFor {
			kept = append(kept, r)
		}
	}
	dropped := len(kept) != len(b.Records)
	b.Records = kept
	return dropped
}

// Clone returns a copy that shares nothing with b.
func (b Book) Clone() Book {
	out := Book{Records: make([]Record, len(b.Records))}
	for i, r := range b.Records {
		r.Images = append([]Image(nil), r.Images...)
		r.Changed = append([]string(nil), r.Changed...)
		if r.Detail != nil {
			d := *r.Detail
			r.Detail = &d
		}
		out.Records[i] = r
	}
	return out
}
