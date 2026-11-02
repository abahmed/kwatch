package dashboard

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/redact"
)

// maxWaitReasons bounds the scheduler reasons listed.
const maxWaitReasons = 8

// failedScheduling is the scheduler's event reason for a pod it could
// not place.
const failedScheduling = "FailedScheduling"

// blockerWords turns the scheduler's reasons (as ParseSchedulerMessage
// returns them) into plain words, matched by their start. The first
// match wins.
var blockerWords = []struct{ prefix, text string }{
	{"Insufficient cpu", "Not enough free CPU"},
	{"Insufficient memory", "Not enough free memory"},
	{"Insufficient ephemeral-storage", "Not enough free disk space"},
	{"Insufficient ", "Not enough of a resource it asks for"},
	{"had untolerated taint", "The node has a taint the pod does not " +
		"tolerate"},
	{"were unschedulable", "The node is cordoned"},
	{"didn't match Pod's node affinity/selector", "The pod's node " +
		"selector or node affinity does not match the node"},
	{"had volume node affinity conflict", "The pod's volume is in " +
		"another zone"},
	{"didn't find available persistent volumes to bind", "No free " +
		"volume fits the pod's claim"},
	{"Too many pods", "The node already runs as many pods as it allows"},
	{"didn't match pod anti-affinity rules", "The pod's anti-affinity " +
		"keeps it away from pods already there"},
	{"didn't match pod affinity rules", "The pod's affinity needs " +
		"pods that are not there"},
	{"didn't match pod topology spread constraints", "The pod's " +
		"spread rules rule the node out"},
	{"didn't have free ports", "The host port the pod needs is taken"},
	{"exceed max volume count", "The node has as many volumes " +
		"attached as it allows"},
}

// waitingOf is why p's pods wait to be placed on a node: the newest
// scheduler message about any of them, with its reasons in plain
// words. Nil when none of p's pods waits for the scheduler; cut counts
// the reasons left out.
func waitingOf(
	model inventory.Reader, p incident.Incident,
) (*Waiting, int) {
	var out *Waiting
	pending := 0
	for _, pod := range podsOf(model, p) {
		e, ok := model.Entity(pod)
		if !ok || !unscheduled(e) {
			continue
		}
		pending++
		message, at := schedulerMessage(model, e)
		if message == "" || (out != nil && !at.After(out.At)) {
			continue
		}
		out = &Waiting{Namespace: pod.Namespace, Pod: pod.Name,
			Message: redact.Evidence(message), At: at}
	}
	if out == nil {
		return nil, 0
	}
	out.Pods = pending
	var cut int
	out.Reasons, out.Nodes, cut = waitReasons(out.Message)
	return out, cut
}

// unscheduled reports a pending pod the scheduler has not placed.
func unscheduled(e inventory.Entity) bool {
	return text(e, kube.AttrPhase) == "Pending" &&
		text(e, kube.ConditionKey("PodScheduled")) == "False"
}

// schedulerMessage is the newest FailedScheduling event about the pod,
// else its PodScheduled condition's message.
func schedulerMessage(
	model inventory.Reader, e inventory.Entity,
) (string, time.Time) {
	var message string
	var at time.Time
	for _, note := range model.Notes(e.ID, time.Time{}) {
		if note.Reason == failedScheduling && !note.At.Before(at) {
			message, at = note.Message, note.At
		}
	}
	if message != "" {
		return message, at
	}
	key := kube.ConditionKey("PodScheduled")
	return text(e, key+kube.AttrConditionMessage),
		timeOf(e, key+kube.AttrConditionSince)
}

// waitReasons are the message's reasons in plain words, most nodes
// first, the node count it names and how many reasons were left out.
func waitReasons(message string) ([]WaitReason, int, int) {
	blockers, total := kube.ParseSchedulerMessage(message)
	out := make([]WaitReason, 0, len(blockers))
	for _, b := range blockers {
		out = append(out, WaitReason{Text: blockerText(b.Reason),
			Reason: b.Reason, Nodes: b.Nodes})
	}
	if len(out) > maxWaitReasons {
		return out[:maxWaitReasons], total, len(out) - maxWaitReasons
	}
	return out, total, 0
}

// blockerText is the reason in plain words, or as the scheduler wrote
// it when kwatch does not know it.
func blockerText(reason string) string {
	for _, w := range blockerWords {
		if strings.HasPrefix(reason, w.prefix) {
			return w.text
		}
	}
	return reason
}
