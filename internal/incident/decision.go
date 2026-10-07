package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Action is what a decision asks delivery to do.
type Action uint8

// Actions.
const (
	Announce Action = iota + 1
	Update
	Resolve
)

// Facts are the data a message quotes beyond the incident itself. The
// pipeline fills them when it sends a decision, and compose reads them
// as data: it never reads the model.
type Facts struct {
	// Output holds application output gathered by investigation after the
	// decision, already redacted. Empty when none was needed or found.
	Output []string
	// Evidence holds the facts investigation found, already redacted.
	// Writers quote them as proof. Empty when none were found.
	Evidence []Fact
	// Changes are the latest changes in the root's namespace, newest
	// first, set for an incident without a cause (see RecentChanges).
	Changes []inventory.Change
	// KindNames are the API's spellings of the custom kinds in the
	// incident ("DatadogAgent" for the kind "datadogagent"), read from
	// the entities' kind.name attribute so a message names a custom
	// resource the way its CRD does.
	KindNames map[inventory.Kind]string
	// Wake is the cluster wake-up the problem began in or right after,
	// set on an announcement; nil when there was none.
	Wake *WakeContext
}

// WakeContext is a cluster wake-up, as a message mentions it: the
// workloads that started from zero replicas, the first and latest start,
// and whether it is still going on.
type WakeContext struct {
	Started  int
	From, To time.Time
	Ongoing  bool
}

// Decision is one message-worthy transition.
type Decision struct {
	Action   Action
	Incident Incident
	// Reason explains why this decision was made, for the audit trail.
	Reason Reason
	// Facts is what the pipeline gathered for the writer after the
	// decision. The writer reads Facts, never the model.
	Facts Facts
	// PagedAlready marks an announcement whose paging alert an earlier
	// paging-only message already opened (a namespace outage hold paged
	// it, then released it as not an outage). It goes to chat only, so
	// the pagers are not told twice.
	PagedAlready bool
	// Unannounced marks a resolve of an incident whose announcement never
	// reached people (a restart lost it while held) but whose page did
	// reach the pagers. It closes that alert and goes to the pagers alone.
	Unannounced bool
	// Handover marks the resolve of a restored incident whose failures
	// another incident took over. Chat hears it as a reply in the old
	// thread, since the restart left that thread open.
	Handover bool
	// Thread marks a decision of a digest-tier incident that fell from a
	// louder tier after people were told in its own thread. The cause
	// update and the resolve belong to that thread, not to a digest.
	Thread bool
}
