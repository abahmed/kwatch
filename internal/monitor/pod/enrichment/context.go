// Package enrichment adds Kubernetes-backed context to Pod findings.
//
// Pure detection policy lives in the sibling policy package. Enrichers may
// read Kubernetes clients, informer listers, events, and logs, but they do
// not decide incident identity or delivery.
package enrichment

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/monitor/pod/policy"
)

// Enricher may add evidence to a finding or suppress it after policy has
// detected a problem.
type Enricher interface {
	Enrich(ctx *Context) (shouldSkip bool)
}

// Context contains the I/O dependencies and mutable evidence used by the
// enrichment stage. Deterministic policy receives a smaller policy.Context.
type Context struct {
	Sources

	Pod    *corev1.Pod
	EvType string
	Owner  *apiv1.OwnerReference
	Events *[]corev1.Event

	policy.Findings
	Container *ContainerContext
}

// Sources are read-only I/O dependencies for enrichment.
type Sources struct {
	Ctx     context.Context
	Client  kubernetes.Interface
	Runtime config.RuntimeConfig

	RSLister      appsv1lister.ReplicaSetLister
	DSLister      appsv1lister.DaemonSetLister
	SSLister      appsv1lister.StatefulSetLister
	EventLister   corev1lister.EventLister
	EventsByPod   func(namespace, pod string) ([]*corev1.Event, error)
	LogCache      *LogCache
	ContainerLogs ContainerLogFetcher
	Now           func() time.Time
}

// ContainerLogFetcher retrieves a bounded log tail for one container. The
// Kubernetes implementation is injected by the composition root so this
// package does not depend on higher-level Kubernetes helpers.
type ContainerLogFetcher func(
	ctx context.Context,
	podName string,
	containerName string,
	namespace string,
	previous bool,
	maxLines int64,
) string

// ContainerContext is the shared policy and enrichment state for one
// container. The alias keeps the package API readable without duplicating the
// structure.
type ContainerContext = model.ContainerContext
