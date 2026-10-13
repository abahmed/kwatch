// Package unexplained measures what kwatch could not explain. In: counts
// of Warning events whose reason it has no entry for, and of incidents
// whose cause stayed unclear, each with the failure mode and the quoted
// reason. Out: seven days of bounded counts, kept in the state file and
// shown on /status as "Unexplained (last 7 days)".
//
// The counts are display only, like the cause self-check: nothing in
// detection, root-cause scoring or messages reads them. They are bounded
// in every direction: a fixed number of mode and reason rows (the rest
// share one overflow row), a few short samples per row, and seven days
// of one number per row. A sample is evidence text that has passed the
// shared redaction, cut short; no Secret data is ever kept.
package unexplained
