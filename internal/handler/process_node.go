package handler

import (
	"fmt"
	"strings"
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
				// Skip alert if Reason is in IgnoreNodeReasons
				for _, ignoreReason := range h.config.IgnoreNodeReasons {
					if c.Reason == ignoreReason {
						logrus.Debugf("Skipping Notify for node %s due to ignored reason: %s", node.Name, c.Reason)
						return
					}
				}
				// Skip alert if Message matches in IgnoreNodeMessages
				for _, ignoreMessage := range h.config.IgnoreNodeMessages {
					if strings.Contains(c.Message, ignoreMessage) {
						logrus.Debugf("Skipping Notify for node %s due to ignored message: %s", node.Name, c.Message)
						return
					}
				}
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
