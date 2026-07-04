package controller

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/resource"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	admissionregistrationv1lister "k8s.io/client-go/listers/admissionregistration/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	policyv1lister "k8s.io/client-go/listers/policy/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

type Controller struct {
	handler                handler.Handler
	podQueue               workqueue.TypedRateLimitingInterface[string]
	nodeQueue              workqueue.TypedRateLimitingInterface[string]
	deploymentQueue        workqueue.TypedRateLimitingInterface[string]
	jobQueue               workqueue.TypedRateLimitingInterface[string]
	daemonSetQueue         workqueue.TypedRateLimitingInterface[string]
	statefulSetQueue       workqueue.TypedRateLimitingInterface[string]
	pdbQueue               workqueue.TypedRateLimitingInterface[string]
	cronJobQueue           workqueue.TypedRateLimitingInterface[string]
	podLister              corev1lister.PodLister
	podsSynced             []cache.InformerSynced
	nodeLister             corev1lister.NodeLister
	nodesSynced            cache.InformerSynced
	deployLister           appsv1lister.DeploymentLister
	deploysSynced          []cache.InformerSynced
	jobLister              batchv1lister.JobLister
	jobsSynced             []cache.InformerSynced
	cronJobLister          batchv1lister.CronJobLister
	cronJobsSynced         []cache.InformerSynced
	rsLister               appsv1lister.ReplicaSetLister
	rsSynced               []cache.InformerSynced
	dsLister               appsv1lister.DaemonSetLister
	dsSynced               []cache.InformerSynced
	ssLister               appsv1lister.StatefulSetLister
	ssSynced               []cache.InformerSynced
	pdbLister              policyv1lister.PodDisruptionBudgetLister
	pdbSynced              cache.InformerSynced
	eventLister            corev1lister.EventLister
	eventsSynced           []cache.InformerSynced
	deploymentWatchEnabled bool
	jobWatchEnabled        bool
	daemonSetWatchEnabled  bool
	statefulSetWatchEnabled bool
	pdbWatchEnabled        bool
	cronJobWatchEnabled    bool
	hpaQueue               workqueue.TypedRateLimitingInterface[string]
	hpaLister              autoscalingv2lister.HorizontalPodAutoscalerLister
	hpaSynced              []cache.InformerSynced
	hpaWatchEnabled        bool
	secretLister           corev1lister.SecretLister
	secretsSynced          []cache.InformerSynced
	maxBaseline            int

	tracker                *kwcontext.ChangeTracker
	graph                  *kwcontext.ResourceGraph
	configMapLister        corev1lister.ConfigMapLister
	configMapSynced        []cache.InformerSynced

	serviceQueue            workqueue.TypedRateLimitingInterface[string]
	endpointQueue           workqueue.TypedRateLimitingInterface[string]
	mwcQueue                workqueue.TypedRateLimitingInterface[string]
	vwcQueue                workqueue.TypedRateLimitingInterface[string]
	ingressQueue            workqueue.TypedRateLimitingInterface[string]
	netpolQueue             workqueue.TypedRateLimitingInterface[string]
	cpPodQueue              workqueue.TypedRateLimitingInterface[string]
	serviceLister           corev1lister.ServiceLister
	svcSynced               []cache.InformerSynced
	endpointLister          corev1lister.EndpointsLister
	endpointSynced          []cache.InformerSynced
	mwcLister               admissionregistrationv1lister.MutatingWebhookConfigurationLister
	mwcSynced               cache.InformerSynced
	vwcLister               admissionregistrationv1lister.ValidatingWebhookConfigurationLister
	vwcSynced               cache.InformerSynced
	ingressLister           networkingv1lister.IngressLister
	ingressSynced           []cache.InformerSynced
	netpolLister            networkingv1lister.NetworkPolicyLister
	netpolSynced            []cache.InformerSynced
	cpPodLister             corev1lister.PodLister
	cpSynced                cache.InformerSynced
	serviceWatchEnabled     bool
	endpointWatchEnabled    bool
	mwcWatchEnabled         bool
	vwcWatchEnabled         bool
	ingressWatchEnabled     bool
	netpolWatchEnabled      bool
	cpWatchEnabled          bool

	nodeResourceCfg        *config.NodeResourceMonitor

	readyFn func()
}

// resolveNamespaces decides which namespaces to watch.
// If NamespaceSelector is set, it lists namespaces via k8s API using the label
// selector. Otherwise it uses the static AllowedNamespaces/ForbiddenNamespaces.
func resolveNamespaces(cfg *config.Config, clientset kubernetes.Interface) ([]string, error) {
	if cfg.NamespaceSelector != "" {
		list, err := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{
			LabelSelector: cfg.NamespaceSelector,
		})
		if err != nil {
			return nil, fmt.Errorf("namespaceSelector list failed: %w", err)
		}
		ns := make([]string, 0, len(list.Items))
		for _, n := range list.Items {
			ns = append(ns, n.Name)
		}
		return ns, nil
	}
	return cfg.AllowedNamespaces, nil
}

func newFactories(client kubernetes.Interface, namespaces []string, resync time.Duration) (factorySet, []informers.SharedInformerFactory) {
	if len(namespaces) <= 1 {
		var opts []informers.SharedInformerOption
		if len(namespaces) == 1 {
			opts = append(opts, informers.WithNamespace(namespaces[0]))
		}
		factory := informers.NewSharedInformerFactoryWithOptions(client, resync, opts...)
		return factorySet{global: factory}, []informers.SharedInformerFactory{factory}
	}

	factories := make([]informers.SharedInformerFactory, 0, len(namespaces))
	for _, ns := range namespaces {
		opts := []informers.SharedInformerOption{informers.WithNamespace(ns)}
		factories = append(factories, informers.NewSharedInformerFactoryWithOptions(client, resync, opts...))
	}
	return factorySet{perNamespace: factories}, factories
}

type factorySet struct {
	global       informers.SharedInformerFactory
	perNamespace []informers.SharedInformerFactory
}

func (fs factorySet) hasMultiple() bool { return len(fs.perNamespace) > 0 }

func (fs factorySet) podLister() corev1lister.PodLister {
	if fs.global != nil {
		return fs.global.Core().V1().Pods().Lister()
	}
	listers := make([]corev1lister.PodLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Core().V1().Pods().Lister())
	}
	return &multiPodLister{listers: listers}
}

func (fs factorySet) podInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Core().V1().Pods().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Core().V1().Pods().Informer())
	}
	return out
}

func (fs factorySet) nodeLister() corev1lister.NodeLister {
	if fs.global != nil {
		return fs.global.Core().V1().Nodes().Lister()
	}
	return fs.perNamespace[0].Core().V1().Nodes().Lister()
}

func (fs factorySet) nodeInformer() cache.SharedIndexInformer {
	if fs.global != nil {
		return fs.global.Core().V1().Nodes().Informer()
	}
	return fs.perNamespace[0].Core().V1().Nodes().Informer()
}

func (fs factorySet) deployLister() appsv1lister.DeploymentLister {
	if fs.global != nil {
		return fs.global.Apps().V1().Deployments().Lister()
	}
	listers := make([]appsv1lister.DeploymentLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Apps().V1().Deployments().Lister())
	}
	return &multiDeploymentLister{listers: listers}
}

func (fs factorySet) deployInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Apps().V1().Deployments().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Apps().V1().Deployments().Informer())
	}
	return out
}

func (fs factorySet) jobLister() batchv1lister.JobLister {
	if fs.global != nil {
		return fs.global.Batch().V1().Jobs().Lister()
	}
	listers := make([]batchv1lister.JobLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Batch().V1().Jobs().Lister())
	}
	return &multiJobLister{listers: listers}
}

func (fs factorySet) rsLister() appsv1lister.ReplicaSetLister {
	if fs.global != nil {
		return fs.global.Apps().V1().ReplicaSets().Lister()
	}
	listers := make([]appsv1lister.ReplicaSetLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Apps().V1().ReplicaSets().Lister())
	}
	return &multiReplicaSetLister{listers: listers}
}

func (fs factorySet) rsInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Apps().V1().ReplicaSets().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Apps().V1().ReplicaSets().Informer())
	}
	return out
}

func (fs factorySet) dsLister() appsv1lister.DaemonSetLister {
	if fs.global != nil {
		return fs.global.Apps().V1().DaemonSets().Lister()
	}
	listers := make([]appsv1lister.DaemonSetLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Apps().V1().DaemonSets().Lister())
	}
	return &multiDaemonSetLister{listers: listers}
}

func (fs factorySet) dsInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Apps().V1().DaemonSets().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Apps().V1().DaemonSets().Informer())
	}
	return out
}

func (fs factorySet) ssLister() appsv1lister.StatefulSetLister {
	if fs.global != nil {
		return fs.global.Apps().V1().StatefulSets().Lister()
	}
	listers := make([]appsv1lister.StatefulSetLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Apps().V1().StatefulSets().Lister())
	}
	return &multiStatefulSetLister{listers: listers}
}

func (fs factorySet) ssInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Apps().V1().StatefulSets().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Apps().V1().StatefulSets().Informer())
	}
	return out
}

func (fs factorySet) pdbLister() policyv1lister.PodDisruptionBudgetLister {
	if fs.global != nil {
		return fs.global.Policy().V1().PodDisruptionBudgets().Lister()
	}
	listers := make([]policyv1lister.PodDisruptionBudgetLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Policy().V1().PodDisruptionBudgets().Lister())
	}
	return &multiPodDisruptionBudgetLister{listers: listers}
}

func (fs factorySet) pdbInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Policy().V1().PodDisruptionBudgets().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Policy().V1().PodDisruptionBudgets().Informer())
	}
	return out
}

func (fs factorySet) jobInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Batch().V1().Jobs().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Batch().V1().Jobs().Informer())
	}
	return out
}

func (fs factorySet) cronJobLister() batchv1lister.CronJobLister {
	if fs.global != nil {
		return fs.global.Batch().V1().CronJobs().Lister()
	}
	listers := make([]batchv1lister.CronJobLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Batch().V1().CronJobs().Lister())
	}
	return &multiCronJobLister{listers: listers}
}

func (fs factorySet) cronJobInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Batch().V1().CronJobs().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Batch().V1().CronJobs().Informer())
	}
	return out
}

func (fs factorySet) hpaLister() autoscalingv2lister.HorizontalPodAutoscalerLister {
	if fs.global != nil {
		return fs.global.Autoscaling().V2().HorizontalPodAutoscalers().Lister()
	}
	listers := make([]autoscalingv2lister.HorizontalPodAutoscalerLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Autoscaling().V2().HorizontalPodAutoscalers().Lister())
	}
	return &multiHorizontalPodAutoscalerLister{listers: listers}
}

func (fs factorySet) hpaInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Autoscaling().V2().HorizontalPodAutoscalers().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Autoscaling().V2().HorizontalPodAutoscalers().Informer())
	}
	return out
}

func (fs factorySet) serviceLister() corev1lister.ServiceLister {
	if fs.global != nil {
		return fs.global.Core().V1().Services().Lister()
	}
	listers := make([]corev1lister.ServiceLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Core().V1().Services().Lister())
	}
	return &multiServiceLister{listers: listers}
}

func (fs factorySet) serviceInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Core().V1().Services().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Core().V1().Services().Informer())
	}
	return out
}

func (fs factorySet) endpointLister() corev1lister.EndpointsLister {
	if fs.global != nil {
		return fs.global.Core().V1().Endpoints().Lister()
	}
	listers := make([]corev1lister.EndpointsLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Core().V1().Endpoints().Lister())
	}
	return &multiEndpointLister{listers: listers}
}

func (fs factorySet) endpointInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Core().V1().Endpoints().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Core().V1().Endpoints().Informer())
	}
	return out
}

func (fs factorySet) ingressLister() networkingv1lister.IngressLister {
	if fs.global != nil {
		return fs.global.Networking().V1().Ingresses().Lister()
	}
	listers := make([]networkingv1lister.IngressLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Networking().V1().Ingresses().Lister())
	}
	return &multiIngressLister{listers: listers}
}

func (fs factorySet) ingressInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Networking().V1().Ingresses().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Networking().V1().Ingresses().Informer())
	}
	return out
}

func (fs factorySet) netpolLister() networkingv1lister.NetworkPolicyLister {
	if fs.global != nil {
		return fs.global.Networking().V1().NetworkPolicies().Lister()
	}
	listers := make([]networkingv1lister.NetworkPolicyLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Networking().V1().NetworkPolicies().Lister())
	}
	return &multiNetpolLister{listers: listers}
}

func (fs factorySet) netpolInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Networking().V1().NetworkPolicies().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Networking().V1().NetworkPolicies().Informer())
	}
	return out
}

func (fs factorySet) configMapLister() corev1lister.ConfigMapLister {
	if fs.global != nil {
		return fs.global.Core().V1().ConfigMaps().Lister()
	}
	listers := make([]corev1lister.ConfigMapLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Core().V1().ConfigMaps().Lister())
	}
	return &multiConfigMapLister{listers: listers}
}

func (fs factorySet) configMapInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Core().V1().ConfigMaps().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Core().V1().ConfigMaps().Informer())
	}
	return out
}

func (fs factorySet) mwcLister() admissionregistrationv1lister.MutatingWebhookConfigurationLister {
	if fs.global != nil {
		return fs.global.Admissionregistration().V1().MutatingWebhookConfigurations().Lister()
	}
	return fs.perNamespace[0].Admissionregistration().V1().MutatingWebhookConfigurations().Lister()
}

func (fs factorySet) mwcInformer() cache.SharedIndexInformer {
	if fs.global != nil {
		return fs.global.Admissionregistration().V1().MutatingWebhookConfigurations().Informer()
	}
	return fs.perNamespace[0].Admissionregistration().V1().MutatingWebhookConfigurations().Informer()
}

func (fs factorySet) vwcLister() admissionregistrationv1lister.ValidatingWebhookConfigurationLister {
	if fs.global != nil {
		return fs.global.Admissionregistration().V1().ValidatingWebhookConfigurations().Lister()
	}
	return fs.perNamespace[0].Admissionregistration().V1().ValidatingWebhookConfigurations().Lister()
}

func (fs factorySet) vwcInformer() cache.SharedIndexInformer {
	if fs.global != nil {
		return fs.global.Admissionregistration().V1().ValidatingWebhookConfigurations().Informer()
	}
	return fs.perNamespace[0].Admissionregistration().V1().ValidatingWebhookConfigurations().Informer()
}

func New(
	client kubernetes.Interface,
	cfg *config.Config,
	h handler.Handler,
) (*Controller, func()) {
	resync := time.Duration(cfg.ResyncSeconds) * time.Second

	namespaces, err := resolveNamespaces(cfg, client)
	if err != nil {
		klog.ErrorS(err, "failed to resolve namespaces")
		os.Exit(1)
	}

	fs, factories := newFactories(client, namespaces, resync)

	podLister := fs.podLister()
	podInformers := fs.podInformers()

	var podsSynced []cache.InformerSynced
	for _, inf := range podInformers {
		podsSynced = append(podsSynced, inf.HasSynced)
	}

	maxBaseline := cfg.Correlation.MaxBaseline
	if maxBaseline <= 0 {
		maxBaseline = correlation.DefaultMaxBaseline
	}

	c := &Controller{
		handler:         h,
		podQueue:        workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "pods"}),
		nodeQueue:       workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "nodes"}),
		deploymentQueue: workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "deployments"}),
		jobQueue:        workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "jobs"}),
		daemonSetQueue:  workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "daemonsets"}),
		statefulSetQueue: workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "statefulsets"}),
		pdbQueue:        workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "poddisruptionbudgets"}),
		cronJobQueue:    workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "cronjobs"}),
		hpaQueue:        workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "horizontalpodautoscalers"}),
		serviceQueue:    workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "services"}),
		endpointQueue:   workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "endpoints"}),
		mwcQueue:        workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "mutatingwebhookconfigurations"}),
		vwcQueue:        workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "validatingwebhookconfigurations"}),
		ingressQueue:    workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "ingresses"}),
		netpolQueue:     workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "networkpolicies"}),
		cpPodQueue:      workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "controlplanepods"}),
		podLister:       podLister,
		podsSynced:      podsSynced,
		maxBaseline:     maxBaseline,
	}

	h.SetPodLister(podLister)

	for _, inf := range podInformers {
		inf.AddEventHandler(c.podEventHandler())
	}

	if cfg.NodeMonitor.Enabled {
		nodeInformer := fs.nodeInformer()
		nodeLister := fs.nodeLister()

		c.nodeLister = nodeLister
		c.nodesSynced = nodeInformer.HasSynced

		h.SetNodeLister(nodeLister)

		nodeInformer.AddEventHandler(c.changeRecordingHandler("node", c.enqueueNode))
	}

	if cfg.RolloutMonitor.Enabled {
		deployLister := fs.deployLister()
		deployInformers := fs.deployInformers()

		c.deployLister = deployLister
		c.deploymentWatchEnabled = true

		var deploysSynced []cache.InformerSynced
		for _, inf := range deployInformers {
			deploysSynced = append(deploysSynced, inf.HasSynced)
		}
		c.deploysSynced = deploysSynced

		h.SetDeploymentLister(deployLister)

		for _, inf := range deployInformers {
			inf.AddEventHandler(c.changeRecordingHandler("deployment", c.enqueueDeployment))
		}
	}

	if cfg.JobMonitor.Enabled {
		jobLister := fs.jobLister()
		jobInformers := fs.jobInformers()

		c.jobLister = jobLister
		c.jobWatchEnabled = true

		var jobsSynced []cache.InformerSynced
		for _, inf := range jobInformers {
			jobsSynced = append(jobsSynced, inf.HasSynced)
		}
		c.jobsSynced = jobsSynced

		h.SetJobLister(jobLister)

		for _, inf := range jobInformers {
			inf.AddEventHandler(c.changeRecordingHandler("job", c.enqueueJob))
		}
	}

	if cfg.DaemonSetMonitor.Enabled {
		dsLister := fs.dsLister()
		dsInformers := fs.dsInformers()

		c.daemonSetWatchEnabled = true

		var dssSynced []cache.InformerSynced
		for _, inf := range dsInformers {
			dssSynced = append(dssSynced, inf.HasSynced)
		}
		c.dsSynced = dssSynced

		h.SetDaemonSetLister(dsLister)

		for _, inf := range dsInformers {
			inf.AddEventHandler(c.changeRecordingHandler("daemonset", c.enqueueDaemonSet))
		}
	}

	if cfg.CronJobMonitor.Enabled {
		c.cronJobLister = fs.cronJobLister()
		cronJobInformers := fs.cronJobInformers()

		c.cronJobWatchEnabled = true

		var cjSynced []cache.InformerSynced
		for _, inf := range cronJobInformers {
			cjSynced = append(cjSynced, inf.HasSynced)
		}
		c.cronJobsSynced = cjSynced

		h.SetCronJobLister(c.cronJobLister)

		for _, inf := range cronJobInformers {
			inf.AddEventHandler(c.changeRecordingHandler("cronjob", c.enqueueCronJob))
		}
	}

	if cfg.HpaMonitor.Enabled {
		c.hpaLister = fs.hpaLister()
		hpaInformers := fs.hpaInformers()

		c.hpaWatchEnabled = true

		var hpaSynced []cache.InformerSynced
		for _, inf := range hpaInformers {
			hpaSynced = append(hpaSynced, inf.HasSynced)
		}
		c.hpaSynced = hpaSynced

		h.SetHorizontalPodAutoscalerLister(c.hpaLister)

		for _, inf := range hpaInformers {
			inf.AddEventHandler(c.changeRecordingHandler("horizontalpodautoscaler", c.enqueueHorizontalPodAutoscaler))
		}
	}

	if cfg.ServiceMonitor.Enabled {
		svcLister := fs.serviceLister()
		svcInformers := fs.serviceInformers()
		epLister := fs.endpointLister()
		epInformers := fs.endpointInformers()

		c.serviceLister = svcLister
		c.endpointLister = epLister
		c.serviceWatchEnabled = true
		c.endpointWatchEnabled = true

		var svcSynced, epSynced []cache.InformerSynced
		for _, inf := range svcInformers {
			svcSynced = append(svcSynced, inf.HasSynced)
		}
		for _, inf := range epInformers {
			epSynced = append(epSynced, inf.HasSynced)
		}
		c.svcSynced = svcSynced
		c.endpointSynced = epSynced

		h.SetServiceLister(svcLister)
		h.SetEndpointLister(epLister)

		for _, inf := range svcInformers {
			inf.AddEventHandler(c.changeRecordingHandler("service", c.enqueueService))
		}
		for _, inf := range epInformers {
			inf.AddEventHandler(c.changeRecordingHandler("endpoint", c.enqueueEndpoint))
		}
	}

	if cfg.AdmissionWebhookMonitor.Enabled {
		mwcLister := fs.mwcLister()
		mwcInformer := fs.mwcInformer()
		vwcLister := fs.vwcLister()
		vwcInformer := fs.vwcInformer()

		c.mwcLister = mwcLister
		c.mwcSynced = mwcInformer.HasSynced
		c.vwcLister = vwcLister
		c.vwcSynced = vwcInformer.HasSynced
		c.mwcWatchEnabled = true
		c.vwcWatchEnabled = true

		h.SetMwCLister(mwcLister)
		h.SetVwCLister(vwcLister)

		mwcInformer.AddEventHandler(c.changeRecordingHandler("mutatingwebhookconfiguration", c.enqueueMwc))
		vwcInformer.AddEventHandler(c.changeRecordingHandler("validatingwebhookconfiguration", c.enqueueVwc))
	}

	if cfg.IngressMonitor.Enabled {
		ingressLister := fs.ingressLister()
		ingressInformers := fs.ingressInformers()

		c.ingressLister = ingressLister
		c.ingressWatchEnabled = true

		var ingressSynced []cache.InformerSynced
		for _, inf := range ingressInformers {
			ingressSynced = append(ingressSynced, inf.HasSynced)
		}
		c.ingressSynced = ingressSynced

		h.SetIngressLister(ingressLister)

		for _, inf := range ingressInformers {
			inf.AddEventHandler(c.changeRecordingHandler("ingress", c.enqueueIngress))
		}
	}

	if cfg.NetworkPolicyMonitor.Enabled {
		netpolLister := fs.netpolLister()
		netpolInformers := fs.netpolInformers()

		c.netpolLister = netpolLister
		c.netpolWatchEnabled = true

		var netpolSynced []cache.InformerSynced
		for _, inf := range netpolInformers {
			netpolSynced = append(netpolSynced, inf.HasSynced)
		}
		c.netpolSynced = netpolSynced

		h.SetNetpolLister(netpolLister)

		for _, inf := range netpolInformers {
			inf.AddEventHandler(c.changeRecordingHandler("networkpolicy", c.enqueueNetpol))
		}
	}

	if cfg.ControlPlaneMonitor.Enabled {
		cpFactory := informers.NewSharedInformerFactoryWithOptions(client, resync, informers.WithNamespace("kube-system"))
		cpPodInformer := cpFactory.Core().V1().Pods().Informer()
		cpPodLister := cpFactory.Core().V1().Pods().Lister()

		c.cpPodLister = cpPodLister
		c.cpSynced = cpPodInformer.HasSynced
		c.cpWatchEnabled = true

		h.SetCpPodLister(cpPodLister)

		cpPodInformer.AddEventHandler(c.changeRecordingHandler("pod", c.enqueueCpPod))

		factories = append(factories, cpFactory)
	}

	{
		c.rsLister = fs.rsLister()

		var rsSynced []cache.InformerSynced
		for _, inf := range fs.rsInformers() {
			rsSynced = append(rsSynced, inf.HasSynced)
		}
		c.rsSynced = rsSynced

		h.SetReplicaLister(c.rsLister)
	}

	{
		c.dsLister = fs.dsLister()

		var dsSynced []cache.InformerSynced
		for _, inf := range fs.dsInformers() {
			dsSynced = append(dsSynced, inf.HasSynced)
		}
		c.dsSynced = dsSynced

		h.SetDaemonSetLister(c.dsLister)
	}

	{
		c.ssLister = fs.ssLister()

		var ssSynced []cache.InformerSynced
		for _, inf := range fs.ssInformers() {
			ssSynced = append(ssSynced, inf.HasSynced)
		}
		c.ssSynced = ssSynced

		h.SetStatefulSetLister(c.ssLister)

		if cfg.StatefulSetMonitor.Enabled {
			c.statefulSetWatchEnabled = true
			for _, inf := range fs.ssInformers() {
				inf.AddEventHandler(c.changeRecordingHandler("statefulset", c.enqueueStatefulSet))
			}
		}
	}

	if cfg.PdbMonitor.Enabled {
		c.pdbLister = fs.pdbLister()
		pdbInformers := fs.pdbInformers()

		c.pdbWatchEnabled = true
		c.pdbSynced = pdbInformers[0].HasSynced

		for _, inf := range pdbInformers {
			inf.AddEventHandler(c.changeRecordingHandler("poddisruptionbudget", c.enqueuePdb))
		}
	}

	// Events informer uses a dedicated factory with field selector to only cache Pod events
	{
		var eventFactories []informers.SharedInformerFactory
		if len(namespaces) <= 1 {
			opts := []informers.SharedInformerOption{
				informers.WithTweakListOptions(func(o *metav1.ListOptions) {
					o.FieldSelector = "involvedObject.kind=Pod"
				}),
			}
			if len(namespaces) == 1 {
				opts = append(opts, informers.WithNamespace(namespaces[0]))
			}
			ef := informers.NewSharedInformerFactoryWithOptions(client, resync, opts...)
			eventFactories = append(eventFactories, ef)
			eventInformer := ef.Core().V1().Events().Informer()
			eventInformer.AddIndexers(cache.Indexers{
				"byPod": func(obj interface{}) ([]string, error) {
					ev, ok := obj.(*corev1.Event)
					if !ok {
						return nil, nil
					}
					return []string{ev.InvolvedObject.Name}, nil
				},
			})
			c.eventLister = ef.Core().V1().Events().Lister()
			c.eventsSynced = append(c.eventsSynced, eventInformer.HasSynced)
		} else {
			listers := make([]corev1lister.EventLister, 0, len(namespaces))
			for _, ns := range namespaces {
				ns := ns
				opts := []informers.SharedInformerOption{
					informers.WithTweakListOptions(func(o *metav1.ListOptions) {
						o.FieldSelector = "involvedObject.kind=Pod"
					}),
					informers.WithNamespace(ns),
				}
				ef := informers.NewSharedInformerFactoryWithOptions(client, resync, opts...)
				eventFactories = append(eventFactories, ef)
				eventInformer := ef.Core().V1().Events().Informer()
				eventInformer.AddIndexers(cache.Indexers{
					"byPod": func(obj interface{}) ([]string, error) {
						ev, ok := obj.(*corev1.Event)
						if !ok {
							return nil, nil
						}
						return []string{ev.InvolvedObject.Name}, nil
					},
				})
				listers = append(listers, ef.Core().V1().Events().Lister())
				c.eventsSynced = append(c.eventsSynced, eventInformer.HasSynced)
			}
			c.eventLister = &multiEventLister{listers: listers}
		}
		h.SetEventLister(c.eventLister)
		factories = append(factories, eventFactories...)
	}

	// ConfigMap informer — always watches monitored namespaces for dependency tracking
	{
		cmLister := fs.configMapLister()
		cmInformers := fs.configMapInformers()

		c.configMapLister = cmLister
		for _, inf := range cmInformers {
			c.configMapSynced = append(c.configMapSynced, inf.HasSynced)
			inf.AddEventHandler(c.changeRecordingHandler("configmap", func(obj interface{}) {
				// ConfigMaps are tracked for dependency analysis only; no queue processing needed
			}))
		}
	}

	if cfg.TlsMonitor.Enabled {
		var tlsFactories []informers.SharedInformerFactory
		if len(namespaces) <= 1 {
			opts := []informers.SharedInformerOption{
				informers.WithTweakListOptions(func(o *metav1.ListOptions) {
					o.FieldSelector = "type=kubernetes.io/tls"
				}),
			}
			if len(namespaces) == 1 {
				opts = append(opts, informers.WithNamespace(namespaces[0]))
			}
			tf := informers.NewSharedInformerFactoryWithOptions(client, resync, opts...)
			tlsFactories = append(tlsFactories, tf)
			c.secretLister = tf.Core().V1().Secrets().Lister()
			c.secretsSynced = append(c.secretsSynced, tf.Core().V1().Secrets().Informer().HasSynced)
		} else {
			listers := make([]corev1lister.SecretLister, 0, len(namespaces))
			for _, ns := range namespaces {
				ns := ns
				opts := []informers.SharedInformerOption{
					informers.WithTweakListOptions(func(o *metav1.ListOptions) {
						o.FieldSelector = "type=kubernetes.io/tls"
					}),
					informers.WithNamespace(ns),
				}
				tf := informers.NewSharedInformerFactoryWithOptions(client, resync, opts...)
				tlsFactories = append(tlsFactories, tf)
				listers = append(listers, tf.Core().V1().Secrets().Lister())
				c.secretsSynced = append(c.secretsSynced, tf.Core().V1().Secrets().Informer().HasSynced)
			}
			c.secretLister = &multiSecretLister{listers: listers}
		}
		h.SetSecretLister(c.secretLister)
		factories = append(factories, tlsFactories...)
	}

	stopCh := make(chan struct{})
	for _, f := range factories {
		f.Start(stopCh)
	}

	if cfg.NodeResourceMonitor.Enabled {
		nc := cfg.NodeResourceMonitor
		c.nodeResourceCfg = &nc
	}

	cleanup := func() {
		close(stopCh)
		for _, f := range factories {
			f.Shutdown()
		}
	}

	return c, cleanup
}

func (c *Controller) SetReadyFunc(fn func()) { c.readyFn = fn }

func (c *Controller) SetTracker(t *kwcontext.ChangeTracker) { c.tracker = t }
func (c *Controller) SetGraph(g *kwcontext.ResourceGraph)   { c.graph = g }

func (c *Controller) enqueuePod(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.podQueue.Add(key)
}

func (c *Controller) enqueueNode(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.nodeQueue.Add(key)
}

func (c *Controller) enqueueDeployment(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.deploymentQueue.Add(key)
}

func (c *Controller) enqueueJob(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.jobQueue.Add(key)
}

func (c *Controller) enqueueDaemonSet(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.daemonSetQueue.Add(key)
}

func (c *Controller) enqueueStatefulSet(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.statefulSetQueue.Add(key)
}

func (c *Controller) enqueuePdb(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.pdbQueue.Add(key)
}

func (c *Controller) enqueueCronJob(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.cronJobQueue.Add(key)
}

func (c *Controller) enqueueHorizontalPodAutoscaler(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.hpaQueue.Add(key)
}

func (c *Controller) recordChange(typ kwcontext.ChangeType, resource string, obj interface{}) {
	if c.tracker == nil {
		return
	}
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	ns, name, _ := cache.SplitMetaNamespaceKey(key)
	if name == "" {
		name = key
	}
	c.tracker.Record(kwcontext.Change{
		Resource:  resource,
		Namespace: ns,
		Name:      name,
		Type:      typ,
		Timestamp: time.Now(),
	})
}

func (c *Controller) changeRecordingHandler(resource string, enqueue func(interface{})) cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.recordChange(kwcontext.ChangeCreate, resource, obj)
			enqueue(obj)
		},
		UpdateFunc: func(old, new interface{}) {
			c.recordChange(kwcontext.ChangeUpdate, resource, new)
			enqueue(new)
		},
		DeleteFunc: func(obj interface{}) {
			c.recordChange(kwcontext.ChangeDelete, resource, obj)
			enqueue(obj)
		},
	}
}

func (c *Controller) addPodToGraph(pod *corev1.Pod) {
	if c.graph == nil {
		return
	}
	ns := pod.Namespace
	name := pod.Name

	if pod.Spec.NodeName != "" {
		c.graph.AddEdge("pod", ns, name, "node", "", pod.Spec.NodeName, "scheduled_on")
	}
	for _, ref := range pod.OwnerReferences {
		ownerKind := strings.ToLower(ref.Kind)
		c.graph.AddEdge("pod", ns, name, ownerKind, ns, ref.Name, "owned_by")
	}
	for _, vol := range pod.Spec.Volumes {
		if cm := vol.ConfigMap; cm != nil {
			c.graph.AddEdge("pod", ns, name, "configmap", ns, cm.Name, "mounts")
		}
		if s := vol.Secret; s != nil {
			c.graph.AddEdge("pod", ns, name, "secret", ns, s.SecretName, "mounts")
		}
		if pvc := vol.PersistentVolumeClaim; pvc != nil {
			c.graph.AddEdge("pod", ns, name, "pvc", ns, pvc.ClaimName, "mounts")
		}
	}
	for _, ctr := range pod.Spec.Containers {
		for _, envFrom := range ctr.EnvFrom {
			if cm := envFrom.ConfigMapRef; cm != nil {
				c.graph.AddEdge("pod", ns, name, "configmap", ns, cm.Name, "env_from")
			}
			if s := envFrom.SecretRef; s != nil {
				c.graph.AddEdge("pod", ns, name, "secret", ns, s.Name, "env_from")
			}
		}
		for _, env := range ctr.Env {
			if env.ValueFrom != nil {
				if cm := env.ValueFrom.ConfigMapKeyRef; cm != nil {
					c.graph.AddEdge("pod", ns, name, "configmap", ns, cm.Name, "env_ref")
				}
				if s := env.ValueFrom.SecretKeyRef; s != nil {
					c.graph.AddEdge("pod", ns, name, "secret", ns, s.Name, "env_ref")
				}
			}
		}
	}

	if c.serviceLister == nil {
		return
	}
	svcs, err := c.serviceLister.Services(ns).List(labels.Everything())
	if err != nil {
		return
	}
	for _, svc := range svcs {
		if svc.Spec.Selector == nil {
			continue
		}
		if labels.SelectorFromSet(svc.Spec.Selector).Matches(labels.Set(pod.Labels)) {
			c.graph.AddEdge("service", ns, svc.Name, "pod", ns, name, "selects")
		}
	}
}

func (c *Controller) removePodFromGraph(pod *corev1.Pod) {
	if c.graph == nil {
		return
	}
	c.graph.RemoveNode("pod", pod.Namespace, pod.Name)
}

func (c *Controller) buildGraph() {
	if c.graph == nil {
		return
	}
	c.graph.Clear()

	pods, err := c.podLister.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "failed to list pods for graph build")
		return
	}
	for _, pod := range pods {
		c.addPodToGraph(pod)
	}

	klog.V(4).InfoS("dependency graph built from informer cache", "edges", len(c.graph.Edges()))
}

func (c *Controller) podEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.recordChange(kwcontext.ChangeCreate, "pod", obj)
			if pod, ok := obj.(*corev1.Pod); ok {
				c.addPodToGraph(pod)
			}
			c.enqueuePod(obj)
		},
		UpdateFunc: func(old, new interface{}) {
			c.recordChange(kwcontext.ChangeUpdate, "pod", new)
			if pod, ok := new.(*corev1.Pod); ok {
				c.removePodFromGraph(pod)
				c.addPodToGraph(pod)
			}
			c.enqueuePod(new)
		},
		DeleteFunc: func(obj interface{}) {
			c.recordChange(kwcontext.ChangeDelete, "pod", obj)
			if pod, ok := obj.(*corev1.Pod); ok {
				c.removePodFromGraph(pod)
			}
			c.enqueuePod(obj)
		},
	}
}

func (c *Controller) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.podQueue.ShutDown()
	defer c.nodeQueue.ShutDown()
	defer c.deploymentQueue.ShutDown()
	defer c.jobQueue.ShutDown()
	defer c.daemonSetQueue.ShutDown()
	defer c.statefulSetQueue.ShutDown()
	defer c.pdbQueue.ShutDown()
	defer c.cronJobQueue.ShutDown()
	defer c.hpaQueue.ShutDown()
	defer c.serviceQueue.ShutDown()
	defer c.endpointQueue.ShutDown()
	defer c.mwcQueue.ShutDown()
	defer c.vwcQueue.ShutDown()
	defer c.ingressQueue.ShutDown()
	defer c.netpolQueue.ShutDown()
	defer c.cpPodQueue.ShutDown()

	klog.InfoS("starting controller")

	klog.InfoS("waiting for informer caches to sync")
	syncFns := make([]cache.InformerSynced, 0,
		1+len(c.podsSynced)+len(c.rsSynced)+len(c.dsSynced)+len(c.ssSynced)+len(c.eventsSynced)+
			len(c.deploysSynced)+len(c.jobsSynced)+len(c.cronJobsSynced)+len(c.secretsSynced)+
			len(c.svcSynced)+len(c.endpointSynced)+len(c.ingressSynced)+len(c.netpolSynced)+3)
	syncFns = append(syncFns, c.podsSynced...)
	syncFns = append(syncFns, c.rsSynced...)
	syncFns = append(syncFns, c.dsSynced...)
	syncFns = append(syncFns, c.ssSynced...)
	syncFns = append(syncFns, c.eventsSynced...)
	if c.nodesSynced != nil {
		syncFns = append(syncFns, c.nodesSynced)
	}
	syncFns = append(syncFns, c.configMapSynced...)
	syncFns = append(syncFns, c.deploysSynced...)
	syncFns = append(syncFns, c.jobsSynced...)
	syncFns = append(syncFns, c.cronJobsSynced...)
	syncFns = append(syncFns, c.hpaSynced...)
	syncFns = append(syncFns, c.secretsSynced...)
	syncFns = append(syncFns, c.svcSynced...)
	syncFns = append(syncFns, c.endpointSynced...)
	if c.mwcSynced != nil {
		syncFns = append(syncFns, c.mwcSynced)
	}
	if c.vwcSynced != nil {
		syncFns = append(syncFns, c.vwcSynced)
	}
	syncFns = append(syncFns, c.ingressSynced...)
	syncFns = append(syncFns, c.netpolSynced...)
	if c.cpSynced != nil {
		syncFns = append(syncFns, c.cpSynced)
	}
	if c.pdbSynced != nil {
		syncFns = append(syncFns, c.pdbSynced)
	}
	if !cache.WaitForCacheSync(ctx.Done(), syncFns...) {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	if c.readyFn != nil {
		c.readyFn()
	}

	c.buildGraph()
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.buildGraph()
			}
		}
	}()
	c.buildSeenSet()
	if c.cpWatchEnabled {
		c.handler.SweepControlPlane()
	}

	if c.nodeResourceCfg != nil {
		go func(cfg *config.NodeResourceMonitor) {
			interval := time.Duration(cfg.IntervalSeconds) * time.Second
			if interval <= 0 {
				interval = 300 * time.Second
			}
			mon := resource.NewMonitor(resource.Config{
				Interval:   interval,
				CpuWarning: cfg.CpuWarning, CpuCritical: cfg.CpuCritical,
				MemWarning: cfg.MemWarning, MemCritical: cfg.MemCritical,
			}, c.nodeLister, c.podLister)
			mon.Run(ctx, func(sig *event.Signal) {
				c.handler.ProcessNodeResourceOvercommit(sig.Reason, sig.NodeName, sig.Hint)
			})
		}(c.nodeResourceCfg)
	}

	klog.InfoS("starting workers")
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.runPodWorker, time.Second)
		if c.nodesSynced != nil {
			go wait.UntilWithContext(ctx, c.runNodeWorker, time.Second)
		}
		if c.deploysSynced != nil {
			go wait.UntilWithContext(ctx, c.runDeploymentWorker, time.Second)
		}
		if c.jobsSynced != nil {
			go wait.UntilWithContext(ctx, c.runJobWorker, time.Second)
		}
		if c.daemonSetWatchEnabled {
			go wait.UntilWithContext(ctx, c.runDaemonSetWorker, time.Second)
		}
		if c.statefulSetWatchEnabled {
			go wait.UntilWithContext(ctx, c.runStatefulSetWorker, time.Second)
		}
		if c.pdbWatchEnabled {
			go wait.UntilWithContext(ctx, c.runPdbWorker, time.Second)
		}
		if c.cronJobWatchEnabled {
			go wait.UntilWithContext(ctx, c.runCronJobWorker, time.Second)
		}
		if c.hpaWatchEnabled {
			go wait.UntilWithContext(ctx, c.runHorizontalPodAutoscalerWorker, time.Second)
		}
		if c.serviceWatchEnabled {
			go wait.UntilWithContext(ctx, c.runServiceWorker, time.Second)
		}
		if c.endpointWatchEnabled {
			go wait.UntilWithContext(ctx, c.runEndpointWorker, time.Second)
		}
		if c.mwcWatchEnabled {
			go wait.UntilWithContext(ctx, c.runMwcWorker, time.Second)
		}
		if c.vwcWatchEnabled {
			go wait.UntilWithContext(ctx, c.runVwcWorker, time.Second)
		}
		if c.ingressWatchEnabled {
			go wait.UntilWithContext(ctx, c.runIngressWorker, time.Second)
		}
		if c.netpolWatchEnabled {
			go wait.UntilWithContext(ctx, c.runNetpolWorker, time.Second)
		}
		if c.cpWatchEnabled {
			go wait.UntilWithContext(ctx, c.runCpPodWorker, time.Second)
		}
	}

	<-ctx.Done()
	klog.InfoS("shutting down workers")
	return nil
}

func (c *Controller) runPodWorker(ctx context.Context) {
	for c.processNextPodItem(ctx) {
	}
}

func (c *Controller) runNodeWorker(ctx context.Context) {
	for c.processNextNodeItem() {
	}
}

func (c *Controller) runDeploymentWorker(ctx context.Context) {
	for c.processNextDeploymentItem() {
	}
}

func (c *Controller) runJobWorker(ctx context.Context) {
	for c.processNextJobItem() {
	}
}

func (c *Controller) runDaemonSetWorker(ctx context.Context) {
	for c.processNextDaemonSetItem() {
	}
}

func (c *Controller) runStatefulSetWorker(ctx context.Context) {
	for c.processNextStatefulSetItem() {
	}
}

func (c *Controller) runPdbWorker(ctx context.Context) {
	for c.processNextPdbItem() {
	}
}

func (c *Controller) runCronJobWorker(ctx context.Context) {
	for c.processNextCronJobItem() {
	}
}

func (c *Controller) runHorizontalPodAutoscalerWorker(ctx context.Context) {
	for c.processNextHorizontalPodAutoscalerItem() {
	}
}

func (c *Controller) runServiceWorker(ctx context.Context) {
	for c.processNextServiceItem() {
	}
}

func (c *Controller) runEndpointWorker(ctx context.Context) {
	for c.processNextEndpointItem() {
	}
}

func (c *Controller) runMwcWorker(ctx context.Context) {
	for c.processNextMwcItem() {
	}
}

func (c *Controller) runVwcWorker(ctx context.Context) {
	for c.processNextVwcItem() {
	}
}

func (c *Controller) runIngressWorker(ctx context.Context) {
	for c.processNextIngressItem() {
	}
}

func (c *Controller) runNetpolWorker(ctx context.Context) {
	for c.processNextNetpolItem() {
	}
}

func (c *Controller) runCpPodWorker(ctx context.Context) {
	for c.processNextCpPodItem() {
	}
}

func (c *Controller) processNextPodItem(ctx context.Context) bool {
	key, quit := c.podQueue.Get()
	if quit {
		return false
	}
	defer c.podQueue.Done(key)

	if err := c.syncPod(ctx, key); err != nil {
		c.podQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing pod %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.podQueue.Forget(key)
	return true
}

func (c *Controller) processNextNodeItem() bool {
	key, quit := c.nodeQueue.Get()
	if quit {
		return false
	}
	defer c.nodeQueue.Done(key)

	if err := c.syncNode(key); err != nil {
		c.nodeQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing node %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.nodeQueue.Forget(key)
	return true
}

func (c *Controller) processNextDeploymentItem() bool {
	key, quit := c.deploymentQueue.Get()
	if quit {
		return false
	}
	defer c.deploymentQueue.Done(key)

	if err := c.syncDeployment(key); err != nil {
		c.deploymentQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing deployment %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.deploymentQueue.Forget(key)
	return true
}

func (c *Controller) processNextJobItem() bool {
	key, quit := c.jobQueue.Get()
	if quit {
		return false
	}
	defer c.jobQueue.Done(key)

	if err := c.syncJob(key); err != nil {
		c.jobQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing job %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.jobQueue.Forget(key)
	return true
}

func (c *Controller) buildSeenSet() {
	pods, err := c.podLister.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "failed to list pods for Seen set")
		return
	}
	now := time.Now()
	baseline := make(map[string]map[string]int64)

	suppressed := map[string]int{}
	total := 0
	add := func(key, pod string) {
		if total >= c.maxBaseline {
			return
		}
		if baseline[key] == nil {
			baseline[key] = map[string]int64{}
		}
		if _, exists := baseline[key][pod]; !exists {
			total++
		}
		baseline[key][pod] = now.Unix()

		// Derive owner/reason key for the suppressed count from the incident key.
		// Incident key format: namespace:owner:reason:container
		if i1 := strings.IndexByte(key, ':'); i1 >= 0 {
			if i2 := strings.LastIndexByte(key, ':'); i2 > i1 {
				ownerReason := key[i1+1 : i2]
				// Replace middle colon with slash: "owner:reason" → "owner/reason"
				if j := strings.IndexByte(ownerReason, ':'); j >= 0 {
					suppressed[ownerReason[:j]+"/"+ownerReason[j+1:]]++
				}
			}
		}
	}

	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodSucceeded {
			continue
		}
		owner := correlation.ResolveOwnerName(pod, c.rsLister, c.dsLister, c.ssLister)
		if owner == "" {
			continue
		}

		statuses := make([]corev1.ContainerStatus, 0,
			len(pod.Status.ContainerStatuses)+len(pod.Status.InitContainerStatuses))
		statuses = append(statuses, pod.Status.ContainerStatuses...)
		statuses = append(statuses, pod.Status.InitContainerStatuses...)

		hadContainerIssue := false
		for _, cs := range statuses {
			var reason string
			if w := cs.State.Waiting; w != nil {
				if w.Reason == "ContainerCreating" || w.Reason == "PodInitializing" {
					continue
				}
				// Match ContainerReasonsFilter: normalize CrashLoopBackOff
				// to LastTerminationState reason so the baseline key matches
				// the live signal key.
				if w.Reason == "CrashLoopBackOff" && cs.LastTerminationState.Terminated != nil {
					reason = cs.LastTerminationState.Terminated.Reason
				} else {
					reason = w.Reason
				}
			} else if t := cs.State.Terminated; t != nil {
				if t.ExitCode == 0 || t.Reason == "Completed" {
					continue
				}
				reason = t.Reason
			} else if cs.State.Running != nil && cs.RestartCount > 0 && cs.LastTerminationState.Terminated != nil {
				reason = cs.LastTerminationState.Terminated.Reason
			}
			if reason == "" {
				continue
			}
			ev := event.Event{Namespace: pod.Namespace, Reason: reason, ContainerName: cs.Name}
			key := correlation.IncidentKey(ev, owner, &model.ContainerState{RestartCount: cs.RestartCount})
			add(key, pod.Name)
			hadContainerIssue = true
		}

		if !hadContainerIssue {
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
					ev := event.Event{Namespace: pod.Namespace, Reason: cond.Reason, ContainerName: "."}
					key := correlation.IncidentKey(ev, owner, nil)
					add(key, pod.Name)
					break
				}
			}
		}
	}

	// Seed alerting node conditions into the baseline
	// and collect broken node names for pre-populating activeNodeIncidents
	var activeNodeIncidents []string
	if c.nodeLister != nil {
		if nodes, err := c.nodeLister.List(labels.Everything()); err == nil {
			for _, n := range nodes {
				hasIssue := false
				for _, cond := range n.Status.Conditions {
					if reason := handler.NodeConditionReason(cond); reason != "" {
						ev := event.Event{Reason: reason}
						key := correlation.IncidentKey(ev, n.Name, nil)
						add(key, n.Name)
						hasIssue = true
					}
				}
				if hasIssue {
					activeNodeIncidents = append(activeNodeIncidents, n.Name)
				}
			}
		}
	}

	// Pre-populate activeNodeIncidents so pod suppression is active
	// before any worker starts (timing race prevention).
	if len(activeNodeIncidents) > 0 {
		c.handler.SetActiveNodeIncidents(activeNodeIncidents)
	}

	// Seed controller resource issues into the baseline.
	// Live owner-level signals set PodName="" (no PodName in process_*.go signals),
	// so we seed under the empty pod key for the baseline match.
	seedSignal := func(sig *event.Signal, name string) {
		ev := event.Event{
			Namespace: sig.Namespace,
			Reason:    sig.Reason,
		}
		key := correlation.IncidentKey(ev, sig.Owner, nil)
		add(key, "") // match the live isBaselined(key, "") lookup
	}

	// DaemonSets
	if c.dsLister != nil {
		if dss, err := c.dsLister.List(labels.Everything()); err == nil {
			for _, ds := range dss {
				if sig := handler.DetectDaemonSetIssue(ds); sig != nil {
					seedSignal(sig, ds.Name)
				}
			}
		}
	}

	// StatefulSets
	if c.ssLister != nil {
		if sss, err := c.ssLister.List(labels.Everything()); err == nil {
			for _, ss := range sss {
				if sig := handler.DetectStatefulSetIssue(ss); sig != nil {
					seedSignal(sig, ss.Name)
				}
			}
		}
	}

	// PodDisruptionBudgets
	if c.pdbLister != nil {
		if pdbs, err := c.pdbLister.List(labels.Everything()); err == nil {
			for _, pdb := range pdbs {
				if sig := handler.DetectPdbIssue(pdb); sig != nil {
					seedSignal(sig, pdb.Name)
				}
			}
		}
	}

	// Deployments
	if c.deployLister != nil {
		if deploys, err := c.deployLister.List(labels.Everything()); err == nil {
			for _, deploy := range deploys {
				if sig := handler.DetectDeploymentIssue(deploy); sig != nil {
					seedSignal(sig, deploy.Name)
				}
			}
		}
	}

	// Jobs
	if c.jobLister != nil {
		if jobs, err := c.jobLister.List(labels.Everything()); err == nil {
			for _, job := range jobs {
				if sig := handler.DetectJobIssue(job); sig != nil {
					seedSignal(sig, job.Name)
				}
			}
		}
	}

	// CronJobs
	if c.cronJobLister != nil {
		if cjs, err := c.cronJobLister.List(labels.Everything()); err == nil {
			for _, cj := range cjs {
				if sig := handler.DetectCronJobIssue(cj); sig != nil {
					seedSignal(sig, cj.Name)
				}
			}
		}
	}

	// HPAs — seed both scaling errors and maxed-out conditions
	if c.hpaLister != nil {
		if hpas, err := c.hpaLister.List(labels.Everything()); err == nil {
			for _, hpa := range hpas {
				for _, sig := range handler.DetectHPAIssues(hpa) {
					seedSignal(sig, hpa.Name)
				}
			}
		}
	}

	// Services — seed service-endpoint issues
	if c.serviceLister != nil && c.endpointLister != nil {
		if svcs, err := c.serviceLister.List(labels.Everything()); err == nil {
			for _, svc := range svcs {
				eps, err := c.endpointLister.Endpoints(svc.Namespace).Get(svc.Name)
				if err != nil {
					continue
				}
				if sig := handler.DetectServiceEndpointIssue(svc, eps); sig != nil {
					seedSignal(sig, svc.Name)
				}
			}
		}
	}

	// Admission webhooks — seed webhook-backend issues
	if c.mwcLister != nil {
		hasSvc := func(ns, name string) bool {
			if c.serviceLister == nil {
				return true
			}
			_, err := c.serviceLister.Services(ns).Get(name)
			return err == nil
		}
		if mwcs, err := c.mwcLister.List(labels.Everything()); err == nil {
			for _, mwc := range mwcs {
				for _, sig := range handler.DetectMutatingWebhookIssue(mwc, hasSvc) {
					seedSignal(sig, mwc.Name)
				}
			}
		}
	}
	if c.vwcLister != nil {
		hasSvc := func(ns, name string) bool {
			if c.serviceLister == nil {
				return true
			}
			_, err := c.serviceLister.Services(ns).Get(name)
			return err == nil
		}
		if vwcs, err := c.vwcLister.List(labels.Everything()); err == nil {
			for _, vwc := range vwcs {
				for _, sig := range handler.DetectValidatingWebhookIssue(vwc, hasSvc) {
					seedSignal(sig, vwc.Name)
				}
			}
		}
	}

	// Ingresses — seed ingress-backend issues
	if c.ingressLister != nil {
		hasSvc := func(ns, name string) bool {
			if c.serviceLister == nil {
				return true
			}
			_, err := c.serviceLister.Services(ns).Get(name)
			return err == nil
		}
		if ings, err := c.ingressLister.List(labels.Everything()); err == nil {
			for _, ing := range ings {
				for _, sig := range handler.DetectIngressIssue(ing, hasSvc) {
					seedSignal(sig, ing.Name)
				}
			}
		}
	}

	// NetworkPolicies — seed restrictive-policy issues
	if c.netpolLister != nil {
		if policies, err := c.netpolLister.List(labels.Everything()); err == nil {
			for _, policy := range policies {
				if sig := handler.DetectNetworkPolicyIssue(policy); sig != nil {
					seedSignal(sig, policy.Name)
				}
			}
		}
	}

	// Control-plane — seed control-plane component failures.
	// Unlike other owner-level signals, CP signals carry PodName, so we
	// seed with the actual pod name (not "") to match the live lookup.
	if c.cpWatchEnabled && c.cpPodLister != nil {
		if pods, err := c.cpPodLister.List(labels.Everything()); err == nil {
			for _, pod := range pods {
				if sig := handler.DetectControlPlanePodIssue(pod); sig != nil {
					ev := event.Event{
						Namespace: sig.Namespace,
						Reason:    sig.Reason,
					}
					key := correlation.IncidentKey(ev, sig.Owner, nil)
					add(key, pod.Name)
				}
			}
		}
	}

	if len(baseline) > 0 {
		klog.V(4).InfoS("Seen set built", "count", len(baseline))
		c.handler.SetSeen(baseline)
	}
	c.handler.ReportStartupSummary(suppressed)
}

func (c *Controller) syncPod(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	pod, err := c.podLister.Pods(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessPod(ctx, key, true)
		}
		return err
	}

	return c.handler.ProcessPodObject(ctx, pod, false)
}

func (c *Controller) syncNode(key string) error {
	deleted := false
	_, err := c.nodeLister.Get(key)
	if err != nil {
		if errors.IsNotFound(err) {
			deleted = true
		} else {
			return err
		}
	}

	return c.handler.ProcessNode(key, deleted)
}

func (c *Controller) syncDeployment(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	deploy, err := c.deployLister.Deployments(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessDeployment(key, true)
		}
		return err
	}

	return c.handler.ProcessDeploymentObject(deploy, false)
}

func (c *Controller) processNextDaemonSetItem() bool {
	key, quit := c.daemonSetQueue.Get()
	if quit {
		return false
	}
	defer c.daemonSetQueue.Done(key)

	if err := c.syncDaemonSet(key); err != nil {
		c.daemonSetQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing daemonset %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.daemonSetQueue.Forget(key)
	return true
}

func (c *Controller) processNextStatefulSetItem() bool {
	key, quit := c.statefulSetQueue.Get()
	if quit {
		return false
	}
	defer c.statefulSetQueue.Done(key)

	if err := c.syncStatefulSet(key); err != nil {
		c.statefulSetQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing statefulset %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.statefulSetQueue.Forget(key)
	return true
}

func (c *Controller) processNextPdbItem() bool {
	key, quit := c.pdbQueue.Get()
	if quit {
		return false
	}
	defer c.pdbQueue.Done(key)

	if err := c.syncPdb(key); err != nil {
		c.pdbQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing pdb %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.pdbQueue.Forget(key)
	return true
}

func (c *Controller) processNextCronJobItem() bool {
	key, quit := c.cronJobQueue.Get()
	if quit {
		return false
	}
	defer c.cronJobQueue.Done(key)

	if err := c.syncCronJob(key); err != nil {
		c.cronJobQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing cronjob %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.cronJobQueue.Forget(key)
	return true
}

func (c *Controller) syncDaemonSet(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	ds, err := c.dsLister.DaemonSets(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessDaemonSet(key, true)
		}
		return err
	}

	return c.handler.ProcessDaemonSetObject(ds, false)
}

func (c *Controller) syncStatefulSet(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	ss, err := c.ssLister.StatefulSets(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessStatefulSet(key, true)
		}
		return err
	}

	return c.handler.ProcessStatefulSetObject(ss, false)
}

func (c *Controller) syncPdb(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	pdb, err := c.pdbLister.PodDisruptionBudgets(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessPdb(key, true)
		}
		return err
	}

	return c.handler.ProcessPdbObject(pdb, false)
}

func (c *Controller) syncCronJob(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	cj, err := c.cronJobLister.CronJobs(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessCronJob(key, true)
		}
		return err
	}

	return c.handler.ProcessCronJobObject(cj, false)
}

func (c *Controller) processNextHorizontalPodAutoscalerItem() bool {
	key, quit := c.hpaQueue.Get()
	if quit {
		return false
	}
	defer c.hpaQueue.Done(key)

	if err := c.syncHorizontalPodAutoscaler(key); err != nil {
		c.hpaQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing hpa %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.hpaQueue.Forget(key)
	return true
}

func (c *Controller) syncHorizontalPodAutoscaler(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	hpa, err := c.hpaLister.HorizontalPodAutoscalers(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessHorizontalPodAutoscaler(key, true)
		}
		return err
	}

	return c.handler.ProcessHorizontalPodAutoscalerObject(hpa, false)
}

func (c *Controller) syncJob(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	job, err := c.jobLister.Jobs(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessJob(key, true)
		}
		return err
	}

	return c.handler.ProcessJobObject(job, false)
}

func (c *Controller) enqueueService(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.serviceQueue.Add(key)
}

func (c *Controller) enqueueEndpoint(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.endpointQueue.Add(key)
}

func (c *Controller) enqueueMwc(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.mwcQueue.Add(key)
}

func (c *Controller) enqueueVwc(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.vwcQueue.Add(key)
}

func (c *Controller) enqueueIngress(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.ingressQueue.Add(key)
}

func (c *Controller) enqueueNetpol(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.netpolQueue.Add(key)
}

func (c *Controller) enqueueCpPod(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.cpPodQueue.Add(key)
}

func (c *Controller) syncService(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	svc, err := c.serviceLister.Services(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessService(key, true)
		}
		return err
	}

	return c.handler.ProcessServiceObject(svc, false)
}

func (c *Controller) syncEndpoint(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	svc, err := c.serviceLister.Services(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return c.handler.ProcessServiceObject(svc, false)
}

func (c *Controller) syncMwc(key string) error {
	mwc, err := c.mwcLister.Get(key)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessMutatingWebhookConfiguration(key, true)
		}
		return err
	}

	return c.handler.ProcessMutatingWebhookConfigurationObject(mwc, false)
}

func (c *Controller) syncVwc(key string) error {
	vwc, err := c.vwcLister.Get(key)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessValidatingWebhookConfiguration(key, true)
		}
		return err
	}

	return c.handler.ProcessValidatingWebhookConfigurationObject(vwc, false)
}

func (c *Controller) syncIngress(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	ing, err := c.ingressLister.Ingresses(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessIngress(key, true)
		}
		return err
	}

	return c.handler.ProcessIngressObject(ing, false)
}

func (c *Controller) syncNetpol(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	policy, err := c.netpolLister.NetworkPolicies(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessNetworkPolicy(key, true)
		}
		return err
	}

	return c.handler.ProcessNetworkPolicyObject(policy, false)
}

func (c *Controller) syncCpPod(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	pod, err := c.cpPodLister.Pods(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return c.handler.ProcessControlPlanePod(pod)
}

func (c *Controller) processNextServiceItem() bool {
	key, quit := c.serviceQueue.Get()
	if quit {
		return false
	}
	defer c.serviceQueue.Done(key)

	if err := c.syncService(key); err != nil {
		c.serviceQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing service %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.serviceQueue.Forget(key)
	return true
}

func (c *Controller) processNextEndpointItem() bool {
	key, quit := c.endpointQueue.Get()
	if quit {
		return false
	}
	defer c.endpointQueue.Done(key)

	if err := c.syncEndpoint(key); err != nil {
		c.endpointQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing endpoint %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.endpointQueue.Forget(key)
	return true
}

func (c *Controller) processNextMwcItem() bool {
	key, quit := c.mwcQueue.Get()
	if quit {
		return false
	}
	defer c.mwcQueue.Done(key)

	if err := c.syncMwc(key); err != nil {
		c.mwcQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing mutatingwebhookconfiguration %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.mwcQueue.Forget(key)
	return true
}

func (c *Controller) processNextVwcItem() bool {
	key, quit := c.vwcQueue.Get()
	if quit {
		return false
	}
	defer c.vwcQueue.Done(key)

	if err := c.syncVwc(key); err != nil {
		c.vwcQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing validatingwebhookconfiguration %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.vwcQueue.Forget(key)
	return true
}

func (c *Controller) processNextIngressItem() bool {
	key, quit := c.ingressQueue.Get()
	if quit {
		return false
	}
	defer c.ingressQueue.Done(key)

	if err := c.syncIngress(key); err != nil {
		c.ingressQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing ingress %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.ingressQueue.Forget(key)
	return true
}

func (c *Controller) processNextNetpolItem() bool {
	key, quit := c.netpolQueue.Get()
	if quit {
		return false
	}
	defer c.netpolQueue.Done(key)

	if err := c.syncNetpol(key); err != nil {
		c.netpolQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing networkpolicy %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.netpolQueue.Forget(key)
	return true
}

func (c *Controller) processNextCpPodItem() bool {
	key, quit := c.cpPodQueue.Get()
	if quit {
		return false
	}
	defer c.cpPodQueue.Done(key)

	if err := c.syncCpPod(key); err != nil {
		c.cpPodQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing controlplane pod %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.cpPodQueue.Forget(key)
	return true
}
