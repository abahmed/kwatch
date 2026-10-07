package detectors

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// maxNodeWhyEvents bounds the node Warning events quoted with a
// NotReady node.
const maxNodeWhyEvents = 2

// maxNodeWhyRunes bounds one quoted condition or event message.
const maxNodeWhyRunes = 140

// echoEvents are node events that only repeat the NotReady itself.
var echoEvents = map[string]bool{
	"NodeNotReady": true, "NodeNotSchedulable": true,
	"NodeSchedulable": true, "NodeReady": true,
}

// nodeWhy is what the node itself says besides the Ready message: the
// Ready status, the other conditions that are True and its recent Warning
// events, each quoted as the node wrote it.
func nodeWhy(
	ctx detection.Context, e inventory.Entity, status string,
) []detection.Evidence {
	out := []detection.Evidence{
		{Label: detection.EvidenceReadyStatus, Value: status}}
	for _, p := range nodePressure {
		out = appendWhy(out, detection.EvidenceNodeCondition, p.condition,
			e, conditionMessage(e, p.condition))
	}
	out = appendWhy(out, detection.EvidenceNodeCondition,
		"NetworkUnavailable", e, conditionMessage(e, "NetworkUnavailable"))
	return append(out, nodeEvents(ctx, e)...)
}

// appendWhy adds a condition's evidence when its status is True.
func appendWhy(
	out []detection.Evidence, label, name string, e inventory.Entity,
	message string,
) []detection.Evidence {
	if s, _, _ := condition(e, name); s != "True" {
		return out
	}
	return append(out, detection.Evidence{Label: label,
		Value: quotedWhy(name, message)})
}

// nodeEvents quotes the newest Warning events of the node, one per
// reason.
func nodeEvents(
	ctx detection.Context, e inventory.Entity,
) []detection.Evidence {
	notes := ctx.Model.Notes(e.ID, ctx.Now.Add(-EventWindow))
	var picked []inventory.Note
	for _, n := range notes {
		if !n.Warning || echoEvents[n.Reason] {
			continue
		}
		picked = replaceOrAdd(picked, n)
	}
	var out []detection.Evidence
	for len(picked) > 0 && len(out) < maxNodeWhyEvents {
		newest := 0
		for i, n := range picked {
			if n.At.After(picked[newest].At) {
				newest = i
			}
		}
		out = append(out, detection.Evidence{
			Label: detection.EvidenceNodeEvent,
			Value: quotedWhy(picked[newest].Reason,
				picked[newest].Message)})
		picked = append(picked[:newest], picked[newest+1:]...)
	}
	return out
}

// replaceOrAdd keeps one note per reason: the newest.
func replaceOrAdd(list []inventory.Note, n inventory.Note) []inventory.Note {
	for i := range list {
		if list[i].Reason == n.Reason {
			if n.At.After(list[i].At) {
				list[i] = n
			}
			return list
		}
	}
	return append(list, n)
}

// quotedWhy words a name and the message it came with: `DiskPressure
// "kubelet has disk pressure"`.
func quotedWhy(name, message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return name
	}
	runes := []rune(message)
	if len(runes) > maxNodeWhyRunes {
		message = string(runes[:maxNodeWhyRunes]) + "…"
	}
	return name + ` "` + message + `"`
}
