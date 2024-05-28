package handler

import (
	"github.com/abahmed/kwatch/alertmanager"
	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/filter"
	"github.com/abahmed/kwatch/storage"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

type Handler interface {
	ProcessPod(evType string, pod *corev1.Pod)
}

type handler struct {
	kclient          kubernetes.Interface
	config           *config.Config
	memory           storage.Storage
	podFilters       []filter.Filter
	containerFilters []filter.Filter
	alertManager     *alertmanager.AlertManager
}

func NewHandler(
	cli kubernetes.Interface,
	cfg *config.Config,
	mem storage.Storage,
	alertManager *alertmanager.AlertManager) Handler {
	// Order is important
	podFilters := []filter.Filter{
		filter.NsFilter{},
		filter.PodStatusFilter{},
		filter.PodEventsFilter{},
		//filter.PodOwnersFilter{},
	}

	containersFilters := []filter.Filter{
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
