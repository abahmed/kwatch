package filter

import (
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
	corev1 "k8s.io/api/core/v1"
	apiv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
)

type Status int

const (
	StatusSkip  Status = iota
	StatusAlert
)

type Detector interface {
	Detect(ctx *Context) Status
}

type Enricher interface {
	Enrich(ctx *Context) (shouldSkip bool)
}

type Filter interface {
	Execute(ctx *Context) (ShouldStop bool)
}

type Context struct {
	Client kubernetes.Interface
	Config *config.Config

	Pod    *corev1.Pod
	EvType string

	Owner    *apiv1.OwnerReference
	Events   *[]corev1.Event
	RSLister appsv1lister.ReplicaSetLister
	DSLister appsv1lister.DaemonSetLister
	SSLister appsv1lister.StatefulSetLister

	PodHasIssues        bool
	ContainersHasIssues bool
	PodReason           string
	PodMsg              string
	PodLastState        *model.ContainerState

	// Container
	Container *ContainerContext
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
}
