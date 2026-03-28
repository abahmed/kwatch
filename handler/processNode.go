package handler

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

func (h *handler) ProcessNode(key string, deleted bool) error {
	name := key

	if deleted {
		h.memory.DelNode(name)
		return nil
	}

	node, err := h.nodeLister.Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.memory.DelNode(name)
			return nil
		}
		return fmt.Errorf("failed to get node %s from cache: %w", name, err)
	}

	return h.ProcessNodeObject(node, false)
}

func (h *handler) ProcessNodeObject(node *corev1.Node, deleted bool) error {
	if node == nil {
		return nil
	}

	if deleted {
		h.memory.DelNode(node.Name)
		return nil
	}

	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			if c.Status == corev1.ConditionFalse && !h.memory.HasNode(node.Name) {
				logrus.Printf("node %s is not ready: %s", node.Name, c.Reason)
				// Skip alert if Reason is in IgnoreNodeReasons
				for _, ignoreReason := range h.config.IgnoreNodeReasons {
					if c.Reason == ignoreReason {
						logrus.Debugf("Skipping Notify for node %s due to ignored reason: %s", node.Name, c.Reason)
						return nil
					}
				}
				// Skip alert if Message matches in IgnoreNodeMessages
				for _, ignoreMessage := range h.config.IgnoreNodeMessages {
					if strings.Contains(c.Message, ignoreMessage) {
						logrus.Debugf("Skipping Notify for node %s due to ignored message: %s", node.Name, c.Message)
						return nil
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
	return nil
}
