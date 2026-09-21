package controller

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/fake"
)

func TestFactoryListersFanOutListAndGet(t *testing.T) {
	set, _ := newFactories(
		fake.NewSimpleClientset(),
		namespaceScope{namespaces: []string{"apps", "system"}}, nil,
		time.Minute,
	)
	selector := labels.Everything()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "pod", Namespace: "apps",
	}}
	_, _ = set.podLister().List(selector)
	_, _ = set.deployLister().List(selector)
	_, _ = set.jobLister().List(selector)
	_, _ = set.rsLister().List(selector)
	_, _ = set.dsLister().List(selector)
	_, _ = set.ssLister().List(selector)
	_, _ = set.pdbLister().List(selector)
	_, _ = set.cronJobLister().List(selector)
	_, _ = set.hpaLister().List(selector)
	_, _ = set.serviceLister().List(selector)
	_, _ = set.endpointSliceLister().List(selector)
	_, _ = set.leaseLister().List(selector)
	_, _ = set.ingressLister().List(selector)
	_, _ = set.netpolLister().List(selector)
	_, _ = set.configMapLister().List(selector)
	_, _ = set.secretLister().List(selector)
	_, _ = set.pvcLister().List(selector)
	_, _ = set.serviceAccountLister().List(selector)
	_, _ = set.resourceQuotaLister().List(selector)
	_, _ = set.limitRangeLister().List(selector)
	_, _ = set.namespaceLister().List(selector)
	_, _ = set.persistentVolumeLister().List(selector)
	_, _ = set.storageClassLister().List(selector)
	_, _ = set.mwcLister().List(selector)
	_, _ = set.vwcLister().List(selector)
	_, _ = set.podLister().Pods("apps").Get("missing")
	_, _ = set.serviceLister().Services("apps").Get("missing")
	_, _ = set.resourceQuotaLister().ResourceQuotas("apps").Get("missing")
	_, _ = set.configMapLister().ConfigMaps("apps").Get("missing")
	_, _ = set.pvcLister().PersistentVolumeClaims("apps").Get("missing")
	_, _ = set.secretLister().Secrets("apps").Get("missing")
	_, _ = set.serviceAccountLister().ServiceAccounts("apps").Get("missing")
	_, _ = set.limitRangeLister().LimitRanges("apps").Get("missing")
	_, _ = set.deployLister().Deployments("apps").Get("missing")
	_, _ = set.endpointSliceLister().EndpointSlices("apps").Get("missing")
	_, _ = set.hpaLister().HorizontalPodAutoscalers("apps").Get("missing")
	_, _ = set.ingressLister().Ingresses("apps").Get("missing")
	_, _ = set.jobLister().Jobs("apps").Get("missing")
	_, _ = set.netpolLister().NetworkPolicies("apps").Get("missing")
	_, _ = set.pdbLister().PodDisruptionBudgets("apps").Get("missing")
	_, _ = set.rsLister().ReplicaSets("apps").Get("missing")
	_, _ = set.ssLister().StatefulSets("apps").Get("missing")
	_, _ = set.dsLister().DaemonSets("apps").Get("missing")
	_, _ = set.cronJobLister().CronJobs("apps").Get("missing")
	_, _ = set.serviceLister().Services("apps").Get("missing")
	_, _ = set.podLister().Pods("apps").Get("missing")
	_, _ = set.pvcLister().PersistentVolumeClaims("apps").Get("missing")
	_, _ = set.jobLister().GetPodJobs(pod)
	_, _ = set.pdbLister().GetPodPodDisruptionBudgets(pod)
	_, _ = set.rsLister().GetPodReplicaSets(pod)
	_, _ = set.dsLister().GetPodDaemonSets(pod)
	_, _ = set.dsLister().GetHistoryDaemonSets(
		&appsv1.ControllerRevision{ObjectMeta: metav1.ObjectMeta{
			Name: "history", Namespace: "apps",
		}},
	)
	_, _ = set.ssLister().GetPodStatefulSets(pod)
}
