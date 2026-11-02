package incident

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// maxRecordBytes is what one incident record may grow to with every
// list the journal and the recurrence memory keep full of long values.
const maxRecordBytes = 96 << 10

// fullJournal fills every journal list to its bound with long values,
// through the functions that bound them.
func fullJournal(p *Incident) {
	long := strings.Repeat("x", 1000)
	var fresh []inventory.Change
	for i := 0; i < 2*maxLinkedChanges; i++ {
		change := inventory.Change{
			Entity: entity(kube.KindDeployment, "d"+string(rune('a'+i))),
			At:     at(time.Duration(i) * time.Second), Actor: long,
			App: long, Cause: long, Revision: long}
		for f := 0; f < 2*maxChangeFields; f++ {
			change.Fields = append(change.Fields, inventory.FieldChange{
				Path: long, Before: long, After: long})
		}
		fresh = append(fresh, change)
	}
	p.Journal.Changes = mergeLinked(nil, fresh, at(0))
	var rows []ContainerState
	for i := 0; i < 2*maxBreakContainers; i++ {
		rows = append(rows, ContainerState{Namespace: "shop",
			Pod: strings.Repeat("p", 253), Container: strings.Repeat("c", 63),
			Restarts: 1, Image: strings.Repeat("i", 300),
			StateReason: "CrashLoopBackOff", LastReason: "OOMKilled"})
	}
	for i := 0; i < maxBreaks; i++ {
		p.Journal.Breaks = append(p.Journal.Breaks, BreakSnapshot{
			At: at(0), Reason: BreakWorse,
			Containers: failingContainers(rows)})
	}
	for i := 0; i < 2*maxTransitions; i++ {
		p.Journal.add(Transition{At: at(0),
			Kind: TransitionFailingAgain, From: "recovering", To: "open"})
	}
}

func TestRecordStaysBoundedWithEveryListFull(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))
	r.m.mu.Lock()
	p := r.m.lookup(entity(kube.KindPod, "web"))
	modes := []detection.Mode{detection.ModeCrashLoop,
		detection.ModeOOMKilled, detection.ModeImagePull}
	for i := 0; i < 2*maxHistory; i++ {
		p.History = appendHistory(p.History, Occurrence{
			Mode: detection.ModeCrashLoop, Modes: modes,
			Opened: at(0), Resolved: at(time.Minute), Fix: FixRollback,
			Heard: true, Trigger: TriggerRollout, Page: true})
		p.Occurrences = appendOccurrence(p.Occurrences, at(0))
	}
	fullJournal(p)
	r.m.mu.Unlock()

	records := r.m.Export()
	require.Len(t, records, 1)
	assert.Len(t, records[0].History, HistoryKept)
	assert.Len(t, records[0].Occurrences, maxOccurrences)
	assert.Len(t, records[0].LinkedChanges, maxLinkedChanges)
	assert.Len(t, records[0].Transitions, maxTransitions)
	body, err := json.Marshal(records[0])
	require.NoError(t, err)
	assert.Less(t, len(body), maxRecordBytes, "record of %d bytes",
		len(body))
}
