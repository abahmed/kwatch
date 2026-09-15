package node

import (
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/monitor"
	"github.com/abahmed/kwatch/internal/observe"
)

// NodeInhibition refreshes the state used to suppress Pod symptoms while a
// node incident is active. It is deliberately smaller than the incident
// engine interface.
type NodeInhibition interface {
	RefreshNodeInhibition(string)
}

// Runtime owns Node queue processing. Detection stays in this package while
// incident lifecycle remains behind the observation sink.
type Runtime struct {
	runtime    config.RuntimeConfig
	sink       monitor.ObservationSink
	inhibition NodeInhibition
	lister     corev1lister.NodeLister
	now        func() time.Time
	mu         sync.Mutex
	firstSeen  map[string]time.Time
	started    bool
	configured bool
}

// ConfigureSources wires the controller's synchronized Node cache once.
func (r *Runtime) ConfigureSources(sources Sources) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return fmt.Errorf("node sources cannot change after processing starts")
	}
	if r.configured {
		return fmt.Errorf("node sources are already configured")
	}
	r.configured = true
	r.lister = sources.Nodes
	return nil
}

// beginProcessing closes the source configuration window. The controller
// wires the lister before workers begin processing queue items.
func (r *Runtime) beginProcessing() {
	r.mu.Lock()
	r.started = true
	r.mu.Unlock()
}

// ProcessNode evaluates one controller queue key.
func (r *Runtime) ProcessNode(key string, deleted bool) error {
	r.beginProcessing()
	r.mu.Lock()
	lister := r.lister
	r.mu.Unlock()
	if lister == nil {
		return nil
	}
	if deleted {
		r.clearNode(key)
		r.resolveNode(key, "")
		return nil
	}
	node, err := lister.Get(key)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.clearNode(key)
			r.resolveNode(key, "")
			return nil
		}
		return fmt.Errorf("get node %s from cache: %w", key, err)
	}
	return r.ProcessNodeObject(node, false)
}

// ProcessNodeObject evaluates a Node object without requiring a lister.
func (r *Runtime) ProcessNodeObject(node *corev1.Node, deleted bool) error {
	r.beginProcessing()
	if node == nil {
		return nil
	}
	if deleted {
		r.clearNode(node.Name)
		r.resolveNode(node.Name, "")
		return nil
	}
	now := r.nowTime()
	for _, condition := range node.Status.Conditions {
		r.processCondition(node, condition, now)
	}
	if finding := DetectDeletionIssue(node, now); finding != nil {
		r.process(finding)
	} else {
		r.resolveNode(node.Name, constant.ReasonNodeStuckTerminating)
	}
	return nil
}

func (r *Runtime) processCondition(
	node *corev1.Node, condition corev1.NodeCondition, now time.Time,
) {
	switch condition.Type {
	case corev1.NodeReady:
		if condition.Status == corev1.ConditionTrue ||
			node.DeletionTimestamp != nil || node.Spec.Unschedulable ||
			IsNew(node, now) {
			r.resolveNode(node.Name, constant.ReasonNodeNotReady)
			return
		}
		r.emit(node, condition, constant.ReasonNodeNotReady)
	case corev1.NodeMemoryPressure, corev1.NodeDiskPressure,
		corev1.NodePIDPressure, corev1.NodeNetworkUnavailable:
		reason := string(condition.Type)
		key := node.Name + "/" + reason
		if condition.Status == corev1.ConditionTrue {
			first := r.mark(key, now)
			sustain := time.Duration(r.sustainedMinutes()) * time.Minute
			if sustain > 0 && now.Sub(first) < sustain {
				return
			}
			r.emit(node, condition, reason)
			return
		}
		r.clear(key)
		r.resolveNode(node.Name, reason)
	}
}

func (r *Runtime) emit(
	node *corev1.Node, condition corev1.NodeCondition, reason string,
) {
	if r.runtime.Compiled() {
		index := r.runtime.SuppressionIndex()
		if filter.MatchesNodeReason(index, condition.Reason) ||
			filter.MatchesNodeMessage(index, condition.Message) {
			klog.V(4).InfoS(
				"node observation suppressed", "component", "node-monitor",
				"operation", "process", "node", node.Name, "reason", reason,
			)
			return
		}
	}
	hint := condition.Reason
	if condition.Message != "" {
		hint += ": " + condition.Message
	}
	obs := observe.Node(node, reason).WithHint(hint)
	r.process(obs)
}

// ProcessNodeResourceOvercommit reports a periodic node-resource finding.
func (r *Runtime) ProcessNodeResourceOvercommit(
	reason, nodeName, hint string, severity model.Severity,
) {
	r.beginProcessing()
	if severity == "" {
		severity = model.SeverityWarning
	}
	r.process(
		observe.NodeNamed(nodeName, reason).
			WithSeverity(severity).WithHint(hint),
	)
}

func (r *Runtime) process(obs *model.Observation) {
	if obs == nil || r.sink == nil {
		return
	}
	obs.IncludeEvents = r.runtime.IncludeEvents()
	obs.IncludeLogs = r.runtime.IncludeLogs()
	r.sink.Process(obs)
}

func (r *Runtime) resolveNode(name, reason string) {
	if r.sink == nil {
		return
	}
	r.sink.Resolve(model.ObjectRef{Kind: "node", Name: name}, reason)
	if r.inhibition != nil {
		r.inhibition.RefreshNodeInhibition(name)
	}
}

func (r *Runtime) nowTime() time.Time {
	r.mu.Lock()
	now := r.now
	r.mu.Unlock()
	if now == nil {
		return time.Time{}
	}
	return now()
}

func (r *Runtime) sustainedMinutes() int {
	return r.runtime.NodeMonitor().SustainedMinutes
}

func (r *Runtime) mark(key string, now time.Time) time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	if first, ok := r.firstSeen[key]; ok {
		return first
	}
	r.firstSeen[key] = now
	return now
}

func (r *Runtime) clear(key string) {
	r.mu.Lock()
	delete(r.firstSeen, key)
	r.mu.Unlock()
}

func (r *Runtime) clearNode(name string) {
	r.mu.Lock()
	for key := range r.firstSeen {
		if len(key) > len(name) && key[:len(name)+1] == name+"/" {
			delete(r.firstSeen, key)
		}
	}
	r.mu.Unlock()
}
