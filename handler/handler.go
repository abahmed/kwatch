package handler

import (
	"github.com/abahmed/kwatch/alertmanager"
	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/filter"
	"github.com/abahmed/kwatch/storage"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

type Handler interface {
	ProcessPod(key string, deleted bool) error
	ProcessNode(key string, deleted bool) error
	ProcessPodObject(pod *corev1.Pod, deleted bool) error
	ProcessNodeObject(node *corev1.Node, deleted bool) error
	SetPodLister(lister corev1lister.PodLister)
	SetNodeLister(lister corev1lister.NodeLister)
}

type handler struct {
	kclient          kubernetes.Interface
	config           *config.Config
	memory           storage.Storage
	podFilters       []filter.Filter
	containerFilters []filter.Filter
	alertManager     *alertmanager.AlertManager
	podLister        corev1lister.PodLister
	nodeLister       corev1lister.NodeLister
}

func NewHandler(
	cli kubernetes.Interface,
	cfg *config.Config,
	mem storage.Storage,
	alertManager *alertmanager.AlertManager) Handler {
	// Order is important
	podFilters := []filter.Filter{
		filter.NamespaceFilter{},
		filter.PodNameFilter{},
		filter.PodStatusFilter{},
		filter.PodEventsFilter{},
		filter.PodOwnersFilter{},
	}

	containersFilters := []filter.Filter{
		filter.NamespaceFilter{},
		filter.PodNameFilter{},
		filter.ContainerNameFilter{},
		filter.ContainerRestartsFilter{},
		filter.ContainerStateFilter{},
		filter.ContainerKillingFilter{},
		filter.ContainerReasonsFilter{},
		filter.ContainerLogsFilter{},
		filter.PodOwnersFilter{},
	}

	return &handler{
		kclient:          cli,
		config:           cfg,
		podFilters:       podFilters,
		containerFilters: containersFilters,
		memory:           mem,
		alertManager:     alertManager,
	}
}

func (h *handler) SetPodLister(lister corev1lister.PodLister) {
	h.podLister = lister
}

func (h *handler) SetNodeLister(lister corev1lister.NodeLister) {
	h.nodeLister = lister
}
