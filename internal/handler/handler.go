package handler

import (
	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/storage"
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
	kclient               kubernetes.Interface
	config                *config.Config
	memory                storage.Storage
	podDetectors          []filter.Detector
	podEnrichers          []filter.Enricher
	containerDetectors    []filter.Detector
	containerEnrichers    []filter.Enricher
	correlator            *correlation.Engine
	alertManager          *alert.AlertManager
	podLister             corev1lister.PodLister
	nodeLister            corev1lister.NodeLister
}

func NewHandler(
	cli kubernetes.Interface,
	cfg *config.Config,
	mem storage.Storage,
	correlator *correlation.Engine,
	alertManager *alert.AlertManager) Handler {
	podDetectors := []filter.Detector{
		filter.NamespaceFilter{},
		filter.PodNameFilter{},
		filter.PodStatusFilter{},
	}

	podEnrichers := []filter.Enricher{
		filter.PodEventsFilter{},
		filter.PodOwnersFilter{},
	}

	containerDetectors := []filter.Detector{
		filter.NamespaceFilter{},
		filter.PodNameFilter{},
		filter.ContainerNameFilter{},
		filter.ContainerRestartsFilter{},
		filter.ContainerStateFilter{},
		filter.ContainerReasonsFilter{},
		filter.NoiseFilter{},
	}

	containerEnrichers := []filter.Enricher{
		filter.ContainerKillingFilter{},
		filter.PodOwnersFilter{},
		filter.ContainerLogsFilter{},
	}

	return &handler{
		kclient:            cli,
		config:             cfg,
		podDetectors:       podDetectors,
		podEnrichers:       podEnrichers,
		containerDetectors: containerDetectors,
		containerEnrichers: containerEnrichers,
		memory:             mem,
		correlator:         correlator,
		alertManager:       alertManager,
	}
}

func (h *handler) SetPodLister(lister corev1lister.PodLister) {
	h.podLister = lister
}

func (h *handler) SetNodeLister(lister corev1lister.NodeLister) {
	h.nodeLister = lister
}
