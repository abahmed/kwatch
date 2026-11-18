package app

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/version"
)

// stopFacts is what a session knew, from the model, about its own pod
// and node when it was asked to stop. It is read from memory only, so
// recording it never delays the shutdown. Its JSON shape is persisted.
type stopFacts struct {
	// Signal is set when the process was asked to stop (SIGTERM).
	Signal       bool `json:"signal,omitempty"`
	NodeCordoned bool `json:"nodeCordoned,omitempty"`
	NodeDeleting bool `json:"nodeDeleting,omitempty"`
	NodeNotReady bool `json:"nodeNotReady,omitempty"`
	// NodeTaint is the key of a taint that marks the node as being
	// drained, shut down or taken back.
	NodeTaint string `json:"nodeTaint,omitempty"`
	// PodDisruption is the reason of the pod's DisruptionTarget
	// condition (Kubernetes 1.26+), such as EvictionByEvictionAPI.
	PodDisruption string `json:"podDisruption,omitempty"`
}

// nodeDisrupted reports whether the node was being taken out of
// service.
func (f stopFacts) nodeDisrupted() bool {
	return f.NodeCordoned || f.NodeDeleting || f.NodeNotReady ||
		f.NodeTaint != ""
}

// stopFactsMaxAge bounds how old the facts from the last pipeline stop
// may be when the session ends: older ones describe an earlier stop.
const stopFactsMaxAge = 5 * time.Minute

// disruptionTaints are taint keys that mark a node as going away, in
// any provider: cordon, node lifecycle, cluster autoscaler, Karpenter
// and the clouds' termination notices.
var disruptionTaints = map[string]bool{
	"node.kubernetes.io/unschedulable":            true,
	"node.kubernetes.io/not-ready":                true,
	"node.kubernetes.io/unreachable":              true,
	"node.kubernetes.io/out-of-service":           true,
	"node.cloudprovider.kubernetes.io/shutdown":   true,
	"ToBeDeletedByClusterAutoscaler":              true,
	"karpenter.sh/disrupted":                      true,
	"karpenter.sh/disruption":                     true,
	"cloud.google.com/impending-node-termination": true,
}

// disruptionWords catch the taints of other tools (termination
// handlers, drainers) by their key.
var disruptionWords = []string{
	"terminat", "disrupt", "tobedeleted", "shutdown", "drain",
}

// disruptionTaint is the first taint in text (the model's AttrTaints)
// that marks the node as going away; "" when none does. A
// PreferNoSchedule taint is only a hint and never counts.
func disruptionTaint(text string) string {
	for _, t := range kube.ParseTaints(text) {
		if t.Effect == "PreferNoSchedule" || t.Key == "" {
			continue
		}
		if disruptionTaints[t.Key] {
			return t.Key
		}
		key := strings.ToLower(t.Key)
		for _, word := range disruptionWords {
			if strings.Contains(key, word) {
				return t.Key
			}
		}
	}
	return ""
}

// attributeReader is the part of the model the stop facts read.
type attributeReader interface {
	Attribute(id inventory.EntityID, name string) (inventory.Attribute, bool)
}

// readStopFacts reads what the model says about kwatch's own pod and
// node. A part the model does not hold is left unset.
func readStopFacts(
	model attributeReader, namespace, pod, node string,
) stopFacts {
	facts := stopFacts{}
	if model == nil {
		return facts
	}
	if node != "" {
		id := inventory.CoreID(kube.KindNode, "", node)
		facts.NodeCordoned = boolAttr(model, id, kube.AttrUnschedulable)
		facts.NodeDeleting = boolAttr(model, id, kube.AttrDeleting)
		if ready, ok := model.Attribute(id, kube.AttrReady); ok {
			isReady, isBool := ready.Value.AsBool()
			facts.NodeNotReady = isBool && !isReady
		}
		if taints, ok := model.Attribute(id, kube.AttrTaints); ok {
			facts.NodeTaint = disruptionTaint(taints.Value.AsText())
		}
	}
	if pod != "" && namespace != "" {
		id := inventory.CoreID(kube.KindPod, namespace, pod)
		key := kube.ConditionKey("DisruptionTarget")
		if status, ok := model.Attribute(id, key); ok &&
			status.Value.AsText() == "True" {
			reason, _ := model.Attribute(id, key+kube.AttrConditionReason)
			facts.PodDisruption = reason.Value.AsText()
			if facts.PodDisruption == "" {
				facts.PodDisruption = "Unknown"
			}
		}
	}
	return facts
}

func boolAttr(
	model attributeReader, id inventory.EntityID, name string,
) bool {
	a, ok := model.Attribute(id, name)
	if !ok {
		return false
	}
	v, isBool := a.Value.AsBool()
	return isBool && v
}

// runIdentity is what the next start compares with the previous
// session: where it runs and what it runs.
type runIdentity struct {
	Pod, Node, Version, Config string
}

// envIdentity is this process's identity from the downward API
// environment and the build.
func envIdentity(config string) runIdentity {
	return runIdentity{
		Pod: os.Getenv("POD_NAME"), Node: os.Getenv("NODE_NAME"),
		Version: version.Short(), Config: config,
	}
}

// configDigest is a short digest of the configuration file, so a
// changed configuration can be told from a plain restart. Only the
// digest is kept; "" when the file cannot be read.
func configDigest() string {
	path := os.Getenv("CONFIG_FILE")
	if path == "" {
		return ""
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- operator config path
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

// podTemplate is the part of a pod name its controller derives from
// the pod template: "kwatch-7d9f8c6b5" of "kwatch-7d9f8c6b5-x2x9z".
// A new template (a rollout) gives new pods a new prefix; a pod
// replaced from the same template keeps it.
func podTemplate(name string) string {
	i := strings.LastIndexByte(name, '-')
	if i <= 0 {
		return name
	}
	return name[:i]
}

func changed(before, after string) bool {
	return before != "" && after != "" && before != after
}

// classifyStop explains a session that stopped on a signal: what its
// own pod and node showed then, else what changed between it and this
// start. "" is a normal restart.
func classifyStop(previous runtimeSession, now runIdentity) string {
	if previous.Stop != nil {
		if reason := disruptionReason(*previous.Stop); reason != "" {
			return reason
		}
	}
	switch {
	case changed(previous.Version, now.Version),
		changed(previous.Config, now.Config),
		changed(podTemplate(previous.PodName), podTemplate(now.Pod)):
		return "deployment_rollout"
	case changed(previous.NodeName, now.Node):
		// Same version and template on another node: the pod was moved.
		return "node_disruption"
	}
	return ""
}

// disruptionReason reads the stop facts: node_disruption when the
// node was going away, eviction when only the pod was evicted or
// preempted.
func disruptionReason(f stopFacts) string {
	switch {
	case f.nodeDisrupted():
		return "node_disruption"
	case f.PodDisruption == "DeletionByTaintManager",
		f.PodDisruption == "DeletionByPodGC":
		return "node_disruption"
	case f.PodDisruption != "":
		return "eviction"
	}
	return ""
}

// identity is this start's identity.
func (s *startupManager) identity() runIdentity {
	id := envIdentity(s.configDigest)
	if s.currentVersion != "" {
		id.Version = s.currentVersion
	}
	return id
}

// noteStop records what the model shows of kwatch's own pod and node
// as the pipeline stops; the session end keeps it (factsAtStop).
func (s *startupManager) noteStop(model attributeReader) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	facts := readStopFacts(model, kubeclient.GetNamespace(),
		s.session.PodName, s.session.NodeName)
	s.stopSeen, s.stopSeenAt = &facts, s.now()
}

// factsAtStop is the stop record of a session asked to stop at "at":
// the signal and the facts of the last pipeline stop, unless those
// are older than stopFactsMaxAge. The caller holds sessionMu.
func (s *startupManager) factsAtStop(at time.Time) *stopFacts {
	facts := stopFacts{Signal: true}
	if s.stopSeen != nil && at.Sub(s.stopSeenAt) <= stopFactsMaxAge {
		facts = *s.stopSeen
		facts.Signal = true
	}
	return &facts
}
