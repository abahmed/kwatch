package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	corev1lister "k8s.io/client-go/listers/core/v1"

	kwcontext "github.com/abahmed/kwatch/internal/context"
)

type failingPodLister struct {
	err error
}

func (l failingPodLister) List(labels.Selector) ([]*corev1.Pod, error) {
	return nil, l.err
}

func (failingPodLister) Pods(string) corev1lister.PodNamespaceLister {
	return nil
}

type failingServiceLister struct {
	err error
}

func (l failingServiceLister) List(labels.Selector) ([]*corev1.Service, error) {
	return nil, l.err
}

func (l failingServiceLister) Services(string) corev1lister.ServiceNamespaceLister {
	return failingServiceNamespaceLister(l)
}

type failingServiceNamespaceLister struct {
	err error
}

func (l failingServiceNamespaceLister) List(labels.Selector) ([]*corev1.Service, error) {
	return nil, l.err
}

func (l failingServiceNamespaceLister) Get(string) (*corev1.Service, error) {
	return nil, l.err
}

// newGraphTestGraph builds a Controller wired with real informer listers backed
// by a fake clientset, plus a fresh ResourceGraph.
func newGraphTestGraph(objects ...runtime.Object) (*Controller, context.CancelFunc) {
	c := &Controller{graph: kwcontext.NewResourceGraph()}
	client := fake.NewSimpleClientset(objects...)
	ctx, cancel := context.WithCancel(context.Background())
	factory := informers.NewSharedInformerFactory(client, 0)
	c.podLister = factory.Core().V1().Pods().Lister()
	c.nodeLister = factory.Core().V1().Nodes().Lister()
	c.serviceLister = factory.Core().V1().Services().Lister()
	c.pvLister = factory.Core().V1().PersistentVolumes().Lister()
	c.pvcLister = factory.Core().V1().PersistentVolumeClaims().Lister()
	c.netpolLister = factory.Networking().V1().NetworkPolicies().Lister()
	c.pdbLister = factory.Policy().V1().PodDisruptionBudgets().Lister()
	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())
	return c, cancel
}

func TestRebuildPodGraphRefreshesSelectorEdges(t *testing.T) {
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "deny", Namespace: "ns1"},
		Spec: networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "web"},
		}},
	}
	budget := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb", Namespace: "ns1"},
		Spec: policyv1.PodDisruptionBudgetSpec{Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "web"},
		}},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1", Labels: map[string]string{"app": "web"}},
	}
	c, cancel := newGraphTestGraph(policy, budget, pod)
	defer cancel()

	c.rebuildPodGraph(pod)

	assert.Equal(t, []string{"pod/ns1/p1"}, c.graph.DependenciesOf("networkpolicy", "ns1", "deny"))
	assert.Equal(t, []string{"pod/ns1/p1"}, c.graph.DependenciesOf("poddisruptionbudget", "ns1", "pdb"))
}

func TestAddPodToGraphExtraKinds(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
		Spec: corev1.PodSpec{
			NodeName:           "n1",
			ServiceAccountName: "sa1",
			ImagePullSecrets:   []corev1.LocalObjectReference{{Name: "pull-secret"}},
			Volumes: []corev1.Volume{{Name: "projected", VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{
					{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "projected-config"}}},
					{Secret: &corev1.SecretProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "projected-secret"}}},
				}},
			}}, {Name: "csi", VolumeSource: corev1.VolumeSource{CSI: &corev1.CSIVolumeSource{Driver: "example.csi"}}}},
		},
	}
	c, cancel := newGraphTestGraph(pod)
	defer cancel()

	c.addPodToGraph(pod)

	assert.ElementsMatch(t, c.graph.DependenciesOf("pod", "ns1", "p1"), []string{
		"node//n1",
		"serviceaccount/ns1/sa1",
		"secret/ns1/pull-secret",
		"configmap/ns1/projected-config",
		"secret/ns1/projected-secret",
		"csidriver//example.csi",
	})
}

func TestRebuildPersistentVolumeClaimStorageClass(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc1", Namespace: "ns1"},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName:       "pv1",
			StorageClassName: strPtr("standard"),
		},
	}
	c, cancel := newGraphTestGraph(pvc)
	defer cancel()

	c.rebuildPersistentVolumeClaim(pvc)

	assert.ElementsMatch(t, c.graph.DependenciesOf("pvc", "ns1", "pvc1"), []string{
		"persistentvolume//pv1",
		"storageclass//standard",
	})
}

func TestRebuildPersistentVolumeNodeAffinity(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{"kubernetes.io/hostname": "n1"}},
	}
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv1"},
		Spec: corev1.PersistentVolumeSpec{
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{Key: "kubernetes.io/hostname", Operator: corev1.NodeSelectorOpIn, Values: []string{"n1"}},
							},
						},
					},
				},
			},
		},
	}
	c, cancel := newGraphTestGraph(node, pv)
	defer cancel()

	c.rebuildPersistentVolume(pv)

	assert.Equal(t, []string{"node//n1"}, c.graph.DependenciesOf("persistentvolume", "", "pv1"))
	assert.Equal(t, []string{"persistentvolume//pv1"}, c.graph.DependentsOf("node", "", "n1"))
}

func TestRebuildIngressTLSSecret(t *testing.T) {
	ing := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "ing", Namespace: "ns"}, Spec: networkingv1.IngressSpec{TLS: []networkingv1.IngressTLS{{SecretName: "tls-cert"}}}}
	c, cancel := newGraphTestGraph()
	defer cancel()
	c.rebuildIngress(ing)
	assert.Contains(t, c.graph.DependenciesOf("ingress", "ns", "ing"), "secret/ns/tls-cert")
}

func TestAddPodToGraphServiceAccountEdge(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
		Spec:       corev1.PodSpec{ServiceAccountName: "sa1"},
	}
	c, cancel := newGraphTestGraph(pod)
	defer cancel()

	c.addPodToGraph(pod)

	assert.Contains(t, c.graph.DependenciesOf("pod", "ns1", "p1"), "serviceaccount/ns1/sa1")
}

func TestBuildGraphPreservesExistingStateWhenPodListFails(t *testing.T) {
	c := &Controller{
		graph:     kwcontext.NewResourceGraph(),
		podLister: failingPodLister{err: errors.New("cache unavailable")},
	}
	c.graph.AddEdge("pod", "ns1", "existing", "node", "", "node1", "scheduled_on")

	c.buildGraph()

	assert.Equal(t, []string{"node//node1"}, c.graph.DependenciesOf("pod", "ns1", "existing"))
}

func TestRebuildPodGraphPreservesExistingEdgesWhenServiceListFails(t *testing.T) {
	c := &Controller{
		graph:         kwcontext.NewResourceGraph(),
		serviceLister: failingServiceLister{err: errors.New("cache unavailable")},
	}
	c.graph.AddEdge("pod", "ns1", "p1", "node", "", "node1", "scheduled_on")
	c.graph.AddEdge("endpointslice", "ns1", "slice1", "pod", "ns1", "p1", "targets")

	c.rebuildPodGraph(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
		Spec:       corev1.PodSpec{NodeName: "node2"},
	})

	assert.Equal(t, []string{"node//node1"}, c.graph.DependenciesOf("pod", "ns1", "p1"))
	assert.Equal(t, []string{"endpointslice/ns1/slice1"}, c.graph.DependentsOf("pod", "ns1", "p1"))
}

func TestRebuildPodGraphReplacesOnlyPodOwnedEdges(t *testing.T) {
	c := &Controller{graph: kwcontext.NewResourceGraph()}
	c.graph.AddEdge("pod", "ns1", "p1", "node", "", "node1", "scheduled_on")
	c.graph.AddEdge("endpointslice", "ns1", "slice1", "pod", "ns1", "p1", "targets")

	c.rebuildPodGraph(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
		Spec:       corev1.PodSpec{NodeName: "node2"},
	})

	assert.Equal(t, []string{"node//node2"}, c.graph.DependenciesOf("pod", "ns1", "p1"))
	assert.Equal(t, []string{"endpointslice/ns1/slice1"}, c.graph.DependentsOf("pod", "ns1", "p1"))
}

func strPtr(s string) *string { return &s }
