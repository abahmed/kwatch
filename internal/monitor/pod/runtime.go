package pod

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/monitor"
	"github.com/abahmed/kwatch/internal/monitor/pod/enrichment"
	"github.com/abahmed/kwatch/internal/observe"
)

// Evaluator is the policy seam between Pod queue lifecycle and detection.
type Evaluator interface {
	EvaluatePod(*enrichment.Context)
	EvaluateContainers(*enrichment.Context)
}

// RecoverySink contains the Pod-specific recovery operations needed after
// evaluation. It deliberately excludes delivery and persistence details.
type RecoverySink interface {
	monitor.ObservationSink
	SetBaseline(map[string]map[string]int64)
	SetActiveNodeIncidents([]string)
	RemovePodWithUID(namespace, podName, podUID string)
	ResolveHealthyPodContainers(
		namespace, podName string, recovered map[string]bool,
	)
	ClearBaselineForPod(namespace, podName string, owner model.ObjectRef)
}

// RuntimeSources are the external inputs used to prepare enrichment data.
// Kubernetes access remains injected, so the family is deterministic in
// unit tests and has no hidden clients.
type RuntimeSources struct {
	Pod            corev1lister.PodLister
	RS             appsv1lister.ReplicaSetLister
	DS             appsv1lister.DaemonSetLister
	SS             appsv1lister.StatefulSetLister
	Events         corev1lister.EventLister
	EventsByPod    func(namespace, pod string) ([]*corev1.Event, error)
	Secret         corev1lister.SecretLister
	ConfigMap      corev1lister.ConfigMapLister
	ServiceAccount corev1lister.ServiceAccountLister
}

// Runtime owns Pod queue lookup and lifecycle recovery. Detection policy is
// supplied through the family-owned Evaluator boundary.
type Runtime struct {
	client        kubernetes.Interface
	runtime       config.RuntimeConfig
	sink          RecoverySink
	evaluator     Evaluator
	containerLogs enrichment.ContainerLogFetcher
	logCache      *enrichment.LogCache
	now           func() time.Time

	mu         sync.Mutex
	sources    RuntimeSources
	started    bool
	configured bool
}

// NewRuntimeWithRuntimeConfig constructs Pod runtime behavior from the
// immutable configuration snapshot.
func NewRuntimeWithRuntimeConfig(
	client kubernetes.Interface,
	runtime config.RuntimeConfig,
	sink RecoverySink,
	evaluator Evaluator,
	containerLogs enrichment.ContainerLogFetcher,
	now func() time.Time,
) *Runtime {
	now = clock.RequireFunc(now)
	return &Runtime{
		client:        client,
		runtime:       runtime,
		sink:          sink,
		evaluator:     evaluator,
		containerLogs: containerLogs,
		logCache:      enrichment.NewLogCache(now),
		now:           now,
	}
}

// ConfigureSources wires the synchronized informer sources once.
func (r *Runtime) ConfigureSources(sources RuntimeSources) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return fmt.Errorf("pod sources cannot change after processing starts")
	}
	if r.configured {
		return fmt.Errorf("pod sources are already configured")
	}
	r.configured = true
	r.sources = sources
	return nil
}

// beginProcessing closes the source configuration window. Sources are wired
// before queue workers start and remain stable for the runtime lifetime.
func (r *Runtime) beginProcessing() {
	r.mu.Lock()
	r.started = true
	r.mu.Unlock()
}

// SetBaseline forwards startup state to the incident sink. Baseline loading is
// an explicit Pod recovery dependency, not an optional runtime capability.
func (r *Runtime) SetBaseline(baseline map[string]map[string]int64) {
	if r.sink != nil {
		r.sink.SetBaseline(baseline)
	}
}

// SetActiveNodeIncidents forwards node inhibition state to the incident sink.
func (r *Runtime) SetActiveNodeIncidents(nodeNames []string) {
	if r.sink != nil {
		r.sink.SetActiveNodeIncidents(nodeNames)
	}
}

// ProcessPod evaluates one Pod queue key.
func (r *Runtime) ProcessPod(
	ctx context.Context,
	key string,
	deleted bool,
) error {
	r.beginProcessing()
	podUID := podUIDFromQueueKey(key)
	if index := strings.LastIndex(key, "#"); index >= 0 {
		key = key[:index]
	}
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid pod key %q: %w", key, err)
	}

	r.mu.Lock()
	sources := r.sources
	r.mu.Unlock()
	if deleted {
		if podUID != "" && sources.Pod != nil {
			current, lookupErr := sources.Pod.Pods(namespace).Get(name)
			if lookupErr == nil && string(current.UID) != podUID {
				return nil
			}
		}
		r.removePod(namespace, name, podUID)
		return nil
	}
	if sources.Pod == nil {
		return nil
	}
	pod, err := sources.Pod.Pods(namespace).Get(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.removePod(namespace, name, podUID)
			return nil
		}
		return fmt.Errorf(
			"failed to get pod %s/%s from cache: %w",
			namespace, name, err,
		)
	}
	return r.ProcessPodObject(ctx, pod, false)
}

// ProcessPodObject evaluates a Pod without requiring a lister lookup.
func (r *Runtime) ProcessPodObject(
	parent context.Context,
	pod *corev1.Pod,
	deleted bool,
) error {
	r.beginProcessing()
	if pod == nil {
		return nil
	}
	if deleted {
		r.removePod(pod.Namespace, pod.Name, string(pod.UID))
		return nil
	}
	r.mu.Lock()
	sources := r.sources
	now := r.now
	r.mu.Unlock()
	now = clock.RequireFunc(now)
	ctx := &enrichment.Context{
		Sources: enrichment.Sources{
			Ctx:           parent,
			Client:        r.client,
			Runtime:       r.runtime,
			RSLister:      sources.RS,
			DSLister:      sources.DS,
			SSLister:      sources.SS,
			EventLister:   sources.Events,
			EventsByPod:   sources.EventsByPod,
			LogCache:      r.logCache,
			ContainerLogs: r.containerLogs,
			Now:           now,
		},
		Pod:    pod,
		EvType: "ADDED",
	}
	if r.evaluator != nil {
		r.evaluator.EvaluatePod(ctx)
		r.evaluator.EvaluateContainers(ctx)
	}
	owner := observe.PodOwners{
		RS: sources.RS,
		DS: sources.DS,
		SS: sources.SS,
	}.OwnerOf(pod)
	if owner.Name == "" {
		owner = observe.SelfOwner("Pod", pod.Namespace, pod.Name)
	}
	for _, observation := range DetectReferenceIssues(pod,
		ReferenceListers{
			Secret: sources.Secret, ConfigMap: sources.ConfigMap,
			ServiceAccount: sources.ServiceAccount,
		},
	) {
		observation.Owner = owner
		r.process(observation)
	}
	if observation := DetectDeletionIssue(pod, now()); observation != nil {
		observation.Owner = owner
		r.process(observation)
	} else if r.sink != nil {
		r.sink.Resolve(
			model.ObjectRef{Kind: "pod", Namespace: pod.Namespace,
				Name: owner.Name},
			constant.ReasonPodStuckTerminating,
		)
	}
	if isHealthy(pod) && r.sink != nil {
		r.sink.ClearBaselineForPod(pod.Namespace, pod.Name, owner)
	}
	if r.sink != nil {
		r.sink.ResolveHealthyPodContainers(
			pod.Namespace, pod.Name, recoveredContainers(pod),
		)
	}
	return nil
}

func (r *Runtime) process(observation *model.Observation) {
	if observation == nil || r.sink == nil {
		return
	}
	observation.IncludeEvents = r.runtime.Monitors().IncludeEvents()
	observation.IncludeLogs = r.runtime.Monitors().IncludeLogs()
	r.sink.Process(observation)
}

func (r *Runtime) removePod(namespace, name, uid string) {
	if r.sink != nil {
		r.sink.RemovePodWithUID(namespace, name, uid)
	}
}

func podUIDFromQueueKey(key string) string {
	for index := len(key) - 1; index >= 0; index-- {
		if key[index] == '#' {
			return key[index+1:]
		}
	}
	return ""
}

func isHealthy(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning &&
		pod.Status.Phase != corev1.PodSucceeded {
		return false
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Waiting != nil &&
			status.State.Waiting.Reason != "ContainerCreating" &&
			status.State.Waiting.Reason != "PodInitializing" {
			return false
		}
		if status.State.Terminated != nil &&
			status.State.Terminated.ExitCode != 0 &&
			status.State.Terminated.Reason != "Completed" {
			return false
		}
	}
	return true
}

func recoveredContainers(pod *corev1.Pod) map[string]bool {
	if pod == nil || pod.DeletionTimestamp != nil ||
		pod.Status.Phase != corev1.PodRunning {
		return nil
	}
	var recovered map[string]bool
	ready := 0
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Running == nil || !status.Ready {
			continue
		}
		if recovered == nil {
			recovered = make(map[string]bool,
				len(pod.Status.ContainerStatuses))
		}
		recovered[status.Name] = true
		ready++
	}
	if ready > 0 && ready == len(pod.Status.ContainerStatuses) {
		recovered[""] = true
	}
	for _, status := range pod.Status.InitContainerStatuses {
		if status.State.Terminated == nil ||
			status.State.Terminated.ExitCode != 0 {
			continue
		}
		if recovered == nil {
			recovered = make(map[string]bool)
		}
		recovered[status.Name] = true
	}
	return recovered
}
