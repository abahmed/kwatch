package controller

import (
	"sync"
	"time"

	admregv1lister "k8s.io/client-go/listers/admissionregistration/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	coordinationv1lister "k8s.io/client-go/listers/coordination/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	policyv1lister "k8s.io/client-go/listers/policy/v1"
	storagev1lister "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

// pipelineSet groups queue ownership by controller responsibility. The
// promoted fields keep sync methods readable while construction remains
// explicit in one place.
type pipelineSet struct {
	pod           *resourcePipeline
	node          *resourcePipeline
	deployment    *resourcePipeline
	job           *resourcePipeline
	daemonSet     *resourcePipeline
	statefulSet   *resourcePipeline
	pdb           *resourcePipeline
	cronJob       *resourcePipeline
	hpa           *resourcePipeline
	service       *resourcePipeline
	endpointSlice *resourcePipeline
	mwc           *resourcePipeline
	vwc           *resourcePipeline
	ingress       *resourcePipeline
	netpol        *resourcePipeline
	cpPod         *resourcePipeline
	resourceQuota *resourcePipeline
	limitRange    *resourcePipeline
	namespace     *resourcePipeline
	lease         *resourcePipeline
	replicaSet    *resourcePipeline
}

type scopeState struct {
	watchAll            bool
	allowedNamespaces   map[string]struct{}
	forbiddenNamespaces map[string]struct{}
}

// sourceSet groups synchronized informer sources by the family that consumes
// them. Embedded source views preserve readable c.podLister-style access while
// making ownership explicit in the controller state.
type sourceSet struct {
	podSources
	workloadSources
	nodeSources
	networkSources
	securitySources
	clusterSources
	integrationSources
	secondarySources
}

type podSources struct {
	podLister   corev1lister.PodLister
	eventLister corev1lister.EventLister
	// one indexer per event informer, all indexed by "byPod".
	eventIndexers []cache.Indexer
	secretLister  corev1lister.SecretLister
}

type workloadSources struct {
	deployLister  appsv1lister.DeploymentLister
	jobLister     batchv1lister.JobLister
	cronJobLister batchv1lister.CronJobLister
	rsLister      appsv1lister.ReplicaSetLister
	dsLister      appsv1lister.DaemonSetLister
	ssLister      appsv1lister.StatefulSetLister
	pdbLister     policyv1lister.PodDisruptionBudgetLister
	hpaLister     autoscalingv2lister.HorizontalPodAutoscalerLister
}

type nodeSources struct {
	nodeLister corev1lister.NodeLister
}

type networkSources struct {
	serviceLister       corev1lister.ServiceLister
	endpointSliceLister discoveryv1lister.EndpointSliceLister
	ingressLister       networkingv1lister.IngressLister
	netpolLister        networkingv1lister.NetworkPolicyLister
}

type securitySources struct {
	mwcLister admregv1lister.MutatingWebhookConfigurationLister
	vwcLister admregv1lister.ValidatingWebhookConfigurationLister
}

type clusterSources struct {
	resourceQuotaLister corev1lister.ResourceQuotaLister
	limitRangeLister    corev1lister.LimitRangeLister
	namespaceLister     corev1lister.NamespaceLister
	leaseLister         coordinationv1lister.LeaseLister
}

type integrationSources struct {
	cpPodLister corev1lister.PodLister
}

type secondarySources struct {
	configMapLister      corev1lister.ConfigMapLister
	configMapSynced      []cache.InformerSynced
	pvcLister            corev1lister.PersistentVolumeClaimLister
	pvLister             corev1lister.PersistentVolumeLister
	serviceAccountLister corev1lister.ServiceAccountLister
	storageClassLister   storagev1lister.StorageClassLister
	rsSynced             []cache.InformerSynced
	dsSynced             []cache.InformerSynced
	ssSynced             []cache.InformerSynced
	eventsSynced         []cache.InformerSynced
	secretsSynced        []cache.InformerSynced
}

type graphRuntime struct {
	tracker     *kwcontext.ChangeTracker
	graph       *kwcontext.ResourceGraph
	graphSynced []cache.InformerSynced
}

type baselineRuntime struct {
	maxBaseline     int
	seedThresholds  seedThresholds
	nodeResourceCfg *config.NodeResourceMonitor
}

type diagnosticState struct {
	informerMu               sync.RWMutex
	informerEvents           int64
	informerWatchErrors      int64
	informerLastEvent        time.Time
	informerLastWatchError   time.Time
	informerLastWatchMessage string
	informers                []cache.SharedIndexInformer
	sourceUnavailable        map[string]bool
}
