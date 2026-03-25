package detector

import (
	"github.com/abahmed/kwatch/internal/event"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

type EventHandler struct {
	pipeline     Pipeline
	alertManager interface {
		NotifyEvent(event.Event)
	}
	k8sClient kubernetes.Interface
}

func NewEventHandler(pipeline Pipeline, alertManager interface{ NotifyEvent(event.Event) }, k8sClient kubernetes.Interface) *EventHandler {
	return &EventHandler{
		pipeline:     pipeline,
		alertManager: alertManager,
		k8sClient:    k8sClient,
	}
}

func (h *EventHandler) ProcessPod(evType string, obj runtime.Object) {
	pod := obj.(*corev1.Pod)

	input := &Input{
		Pod:       pod,
		EventType: evType,
		Client:    h.k8sClient,
	}

	e := h.pipeline.ProcessPod(input)
	if e != nil && h.alertManager != nil {
		h.alertManager.NotifyEvent(event.Event{
			PodName:       e.Name,
			ContainerName: e.Container,
			Namespace:     e.Namespace,
			NodeName:      e.Node,
			Reason:        e.Reason,
			Events:        e.Events,
			Logs:          e.Logs,
			Labels:        e.Labels,
		})
	}
}

func (h *EventHandler) ProcessNode(evType string, obj runtime.Object) {
	node := obj.(*corev1.Node)

	input := &Input{
		Node:      node,
		EventType: evType,
		Client:    h.k8sClient,
	}

	e := h.pipeline.ProcessNode(input)
	if e != nil && h.alertManager != nil {
		h.alertManager.NotifyEvent(event.Event{
			PodName:       e.Name,
			ContainerName: e.Container,
			Namespace:     e.Namespace,
			NodeName:      e.Node,
			Reason:        e.Reason,
			Events:        e.Events,
			Logs:          e.Logs,
			Labels:        e.Labels,
		})
	}
}
