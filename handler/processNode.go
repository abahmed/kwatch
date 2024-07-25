package handler

import (
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func (h *handler) ProcessNode(eventType string, obj runtime.Object) {
	if obj == nil {
		return
	}

	node, ok := obj.(*corev1.Node)
	if !ok {
		logrus.Warnf("failed to cast event to node object: %v", obj)
		return
	}

	if eventType == "DELETED" {
		h.memory.DelNode(node.Name)
		return
	}

	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			if c.Status == corev1.ConditionFalse && !h.memory.HasNode(node.Name) {
				logrus.Printf("node %s is not ready: %s", node.Name, c.Reason)
				h.alertManager.Notify(fmt.Sprintf("Node %s is not ready: %s - %s",
					node.Name,
					c.Reason,
					c.Message,
				))
				h.memory.AddNode(node.Name)
			} else if c.Status == corev1.ConditionTrue {
				h.memory.DelNode(node.Name)
			}
		}
	}

}
