package investigate

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
)

// memoryContextFacts are the labels of the context an OOM kill or a
// memory eviction carries, with how the Proof tab names them.
var memoryContextFacts = []struct{ label, title string }{
	{detection.EvidenceOOMKind, "Limit reached"},
	{detection.EvidenceMemoryHistory, "Memory history"},
	{detection.EvidenceMemoryWindow, "History kept"},
	{detection.EvidenceMemoryPeak, "Memory peak in that history"},
	{detection.EvidenceMemoryLimit, "Memory limit"},
	{detection.EvidenceMemoryRequest, "Memory request"},
	{detection.EvidenceMemoryFlat, "Memory before the kill"},
	{detection.EvidenceMemoryRise, "Memory before the kill"},
	{detection.EvidenceMemoryNearLimit, "Hours near the limit"},
	{detection.EvidenceMemoryRuns, "Last runs ended"},
	{detection.EvidenceNodeMemory, "Node memory"},
	{detection.EvidenceOverRequest, "Use against request"},
	{detection.EvidenceMemoryMaxAllowed, "LimitRange max per container"},
	{detection.EvidenceMemoryQuotaLeft, "Quota limits.memory left"},
	{detection.EvidenceMemoryNodeMax, "Largest node memory"},
	{detection.EvidenceBoundsUnread, "Not readable"},
}

// memoryContextSections are one section per member killed or evicted
// for memory, with the context its finding carries: the history, which
// limit was reached, the node's memory and the bounds of a new limit.
func memoryContextSections(p incident.Incident) []incident.Section {
	var out []incident.Section
	for _, m := range sortedMembers(p) {
		if m.Reason != reasons.OOMKilled && m.Reason != reasons.Evicted {
			continue
		}
		sec := newSection(m.Entity.Name + ": at the kill")
		for _, f := range memoryContextFacts {
			sec.fact(f.title, evidenceValue(m, f.label))
		}
		for _, e := range m.Evidence {
			if e.Label == detection.EvidenceNodeMemoryUser {
				sec.fact("Node memory user", e.Value)
			}
		}
		if len(sec.Facts) > 0 {
			out = append(out, sec.done())
		}
	}
	return out
}
