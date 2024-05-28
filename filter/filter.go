package filter

import (
	"time"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/storage"
	corev1 "k8s.io/api/core/v1"
	apiv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Filter interface {
	Execute(ctx *Context) (ShouldStop bool)
}

type FilterResult struct {
	ShouldStop bool
}

type Context struct {
	Client kubernetes.Interface
	Config *config.Config
	Memory storage.Storage

	Pod    *corev1.Pod
	EvType string

	Owner  *apiv1.OwnerReference
	Events *[]corev1.Event

	PodHasIssues        bool
	ContainersHasIssues bool
	PodReason           string
	PodMsg              string

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
}
