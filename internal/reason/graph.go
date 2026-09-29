package reason

import (
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

// PodOf returns the pod an entity belongs to: the pod itself, or the pod a
// container is part of.
func PodOf(model knowledge.Reader, id knowledge.EntityID) (
	knowledge.EntityID, bool,
) {
	switch id.Kind {
	case kube.KindPod:
		return id, true
	case kube.KindContainer:
		pods := model.Related(id, knowledge.PartOf, knowledge.Outgoing)
		if len(pods) > 0 {
			return pods[0], true
		}
	}
	return knowledge.EntityID{}, false
}

// ownerChain walks owned-by from id to the top controller, id excluded.
func ownerChain(
	model knowledge.Reader, id knowledge.EntityID,
) []knowledge.EntityID {
	var chain []knowledge.EntityID
	seen := map[knowledge.EntityID]bool{id: true}
	for current := id; ; {
		owners := model.Related(
			current, knowledge.OwnedBy, knowledge.Outgoing)
		// Static pods are "owned" by their Node; the node is where they
		// run, not a controller that stamps them out.
		if len(owners) == 0 || seen[owners[0]] ||
			owners[0].Kind == kube.KindNode {
			return chain
		}
		current = owners[0]
		seen[current] = true
		chain = append(chain, current)
	}
}

// TopOwner is the controller that ultimately owns id, or id itself.
func TopOwner(
	model knowledge.Reader, id knowledge.EntityID,
) knowledge.EntityID {
	chain := ownerChain(model, id)
	if len(chain) == 0 {
		return id
	}
	return chain[len(chain)-1]
}

// podsOnNode lists the pods scheduled on node.
func podsOnNode(
	model knowledge.Reader, node knowledge.EntityID,
) []knowledge.EntityID {
	return model.Related(node, knowledge.RunsOn, knowledge.Incoming)
}

// siblingPods lists the other pods owned by the same top controller.
func siblingPods(
	model knowledge.Reader, pod knowledge.EntityID,
) []knowledge.EntityID {
	top := TopOwner(model, pod)
	if top == pod {
		return nil
	}
	var out []knowledge.EntityID
	collectPods(model, top, pod, map[knowledge.EntityID]bool{}, &out)
	return out
}

func collectPods(
	model knowledge.Reader, owner, exclude knowledge.EntityID,
	seen map[knowledge.EntityID]bool, out *[]knowledge.EntityID,
) {
	if seen[owner] {
		return
	}
	seen[owner] = true
	for _, child := range model.Related(
		owner, knowledge.OwnedBy, knowledge.Incoming,
	) {
		if child.Kind == kube.KindPod {
			if child != exclude {
				*out = append(*out, child)
			}
			continue
		}
		collectPods(model, child, exclude, seen, out)
	}
}

// podFailing reports whether a pod or any of its containers has a signal.
func podFailing(q Query, pod knowledge.EntityID) bool {
	if q.Signals.Has(pod) {
		return true
	}
	for _, container := range q.Model.Related(
		pod, knowledge.PartOf, knowledge.Incoming,
	) {
		if q.Signals.Has(container) {
			return true
		}
	}
	return false
}

// failingShare returns how many of pods are failing and the total.
func failingShare(q Query, pods []knowledge.EntityID) (int, int) {
	failing := 0
	for _, pod := range pods {
		if podFailing(q, pod) {
			failing++
		}
	}
	return failing, len(pods)
}

// OwnedPods lists every pod a controller owns, directly or through
// intermediate controllers such as ReplicaSets.
func OwnedPods(
	model knowledge.Reader, owner knowledge.EntityID,
) []knowledge.EntityID {
	if owner.Kind == kube.KindPod {
		return []knowledge.EntityID{owner}
	}
	var out []knowledge.EntityID
	collectPods(model, owner, knowledge.EntityID{},
		map[knowledge.EntityID]bool{}, &out)
	return out
}
