package filter

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
)

type Status int

const (
	StatusSkip Status = iota
	StatusAlert
	StatusContinue
)

type Detector interface {
	Detect(ctx *Context) Status
}

type Enricher interface {
	Enrich(ctx *Context) (shouldSkip bool)
}

// Context is the working state of one pod evaluation. It is composed of
// three parts so a reader can tell dependencies from scratch space:
//
//   - Sources: what filters may look things up in. Set once by the handler,
//     never written by a filter.
//   - the object under evaluation (Pod, EvType, Owner, Events);
//   - Findings: what the detectors concluded. Written by filters, read by the
//     handler to decide whether and what to signal.
//
// The parts are embedded, so ctx.Config and ctx.PodReason read as before.
type Context struct {
	Sources

	Pod    *corev1.Pod
	EvType string
	Owner  *apiv1.OwnerReference
	Events *[]corev1.Event

	Findings

	// Container is the container currently under evaluation, nil during
	// pod-level detection.
	Container *ContainerContext
}

// Sources are the read-only lookups available to filters.
type Sources struct {
	Ctx    context.Context
	Client kubernetes.Interface
	Config *config.Config

	RSLister    appsv1lister.ReplicaSetLister
	DSLister    appsv1lister.DaemonSetLister
	SSLister    appsv1lister.StatefulSetLister
	EventLister corev1lister.EventLister
	// EventsByPod answers "events for this pod" from an index. Preferred over
	// EventLister, which can only list a whole namespace and leaves the
	// filtering to the caller on every alert.
	EventsByPod func(namespace, pod string) ([]*corev1.Event, error)

	// Now is the clock every time-based decision in the filters reads.
	// Injected so "has this pod been unready for 5 minutes" can be tested
	// without waiting 5 minutes; nil means the wall clock.
	Now func() time.Time
}

// Findings are the detectors' conclusions about the pod.
type Findings struct {
	PodHasIssues        bool
	ContainersHasIssues bool
	PodReason           string
	PodMsg              string
	PodLastState        *model.ContainerState
}

type ContainerContext struct {
	Container        *corev1.ContainerStatus
	Reason           string
	Msg              string
	ExitCode         int32
	Logs             string
	HasRestarts      bool
	LastTerminatedOn time.Time
	State            string
	Status           string
	LastState        *model.ContainerState
	IsInit           bool
}

// now returns the injected clock's time, or the wall clock when none is set.
func (c *Context) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}
