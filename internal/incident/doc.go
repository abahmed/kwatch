// Package incident groups findings that share a root cause into one
// Incident and decides when people should hear about it.
// In: Finding transitions and their Causes. Out: Decisions (announce,
// update, resolve) that the notification layer turns into messages.
// The Manager owns every incident's lifecycle, including flapping.
//
// Vocabulary: an incident keeps its root cause as a
// rootcause.CauseRecord. A Fact is something investigation found after
// the incident opened, such as the first error line of a crashed
// container; decisions carry them for writers to quote.
package incident
