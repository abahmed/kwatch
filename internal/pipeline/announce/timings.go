package announce

import "time"

// The collecting steps' timings and thresholds, in one place.
//
// Namespace outages: a namespace whose workloads fail together is
// announced as one message. Each incident still settles on its own clock
// (incident.DefaultSettle, 75s); the outage hold waits for the namespace
// to have none left settling, and never longer than outageHoldMax after
// the first held announcement, so the hold adds at most 30s on top of the
// incident settle. outageWindow decides which incidents opened "together".
const (
	// outageHoldMax is the longest a namespace's announcements wait for
	// the rest of an outage to settle. Each incident settles on its own
	// clock, so failures a few seconds apart are announced a few seconds
	// apart.
	outageHoldMax = 30 * time.Second
	// outageWindow is how close together the incidents must have opened.
	outageWindow = 10 * time.Minute
	// outageIncidents is how many incidents make an outage on their own.
	outageIncidents = 5
	// outageMinShare is the fewest incidents that make an outage when
	// they are at least half of the namespace's workloads.
	outageMinShare = 3
)
