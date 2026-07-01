package handler

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
)

func (h *handler) ProcessNode(key string, deleted bool) error {
	name := key

	if deleted {
		h.correlator.ResolveByResource("node", name)
		return nil
	}

	node, err := h.nodeLister.Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("node", name)
			return nil
		}
		return fmt.Errorf("failed to get node %s from cache: %w", name, err)
	}

	return h.ProcessNodeObject(node, false)
}

// NodeConditionReason returns the stable incident reason for a node
// condition, or "" if the condition is not one kwatch alerts on.
func NodeConditionReason(c corev1.NodeCondition) string {
	switch c.Type {
	case corev1.NodeReady:
		if c.Status != corev1.ConditionTrue {
			return "NodeNotReady"
		}
	case corev1.NodeMemoryPressure, corev1.NodeDiskPressure,
		corev1.NodePIDPressure, corev1.NodeNetworkUnavailable:
		if c.Status == corev1.ConditionTrue {
			return string(c.Type)
		}
	}
	return ""
}

func (h *handler) resolveNodeCondition(nodeName, stableReason string) {
	h.correlator.MarkResolved(correlation.BuildKey("", nodeName, stableReason, ""))
}

func (h *handler) emitNodeAlert(node *corev1.Node, c corev1.NodeCondition, stableReason string) {
	for _, ignoreReason := range h.config.Suppression.NodeReasons {
		if c.Reason == ignoreReason {
			klog.V(4).InfoS("Skipping Notify for node due to ignored reason", "node", node.Name, "reason", c.Reason)
			return
		}
	}
	for _, ignoreMessage := range h.config.Suppression.NodeMessages {
		if strings.Contains(c.Message, ignoreMessage) {
			klog.V(4).InfoS("Skipping Notify for node due to ignored message", "node", node.Name, "message", c.Message)
			return
		}
	}

	hint := ""
	if c.Reason != "" || c.Message != "" {
		hint = c.Reason + ": " + c.Message
	}

	h.signalEvent(&event.Signal{
		Resource: "node",
		PodName:  node.Name,
		NodeName: node.Name,
		Reason:   stableReason,
		Owner:    node.Name,
		Labels:   node.Labels,
		Hint:     hint,
	})
}

func (h *handler) ProcessNodeObject(node *corev1.Node, deleted bool) error {
	if node == nil {
		return nil
	}

	if deleted {
		h.correlator.ResolveByResource("node", node.Name)
		return nil
	}

	for _, c := range node.Status.Conditions {
		switch c.Type {
		case corev1.NodeReady:
			if c.Status == corev1.ConditionTrue {
				h.resolveNodeCondition(node.Name, "NodeNotReady")
			} else if node.DeletionTimestamp != nil || node.Spec.Unschedulable {
                // Node is being intentionally removed (deleting or cordoned) — NodeReady
                // going false is an expected graceful shutdown, not a failure. Uses core
                // Kubernetes signals only (autoscaler-agnostic). Clear any incident, don't alert.
                h.resolveNodeCondition(node.Name, "NodeNotReady")
            } else {
				h.emitNodeAlert(node, c, "NodeNotReady")
			}
		case corev1.NodeMemoryPressure, corev1.NodeDiskPressure,
			corev1.NodePIDPressure, corev1.NodeNetworkUnavailable:
			if c.Status == corev1.ConditionTrue {
				h.emitNodeAlert(node, c, string(c.Type))
			} else {
				h.resolveNodeCondition(node.Name, string(c.Type))
			}
		}
	}
	return nil
}
