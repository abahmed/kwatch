// Package reason finds the root cause of a signal.
//
// Rules propose hypotheses by walking the knowledge graph upstream from a
// symptom. A hypothesis names a root (an unhealthy entity, a change, or
// both) and carries weighted evidence for and against it. Reachability is
// never enough: every rule requires the candidate to be unhealthy or to
// have changed. The engine ranks hypotheses, keeps the best above a
// confidence floor, and records the rest as a reasoning trace.
package reason
