package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// reachCluster is a model holding pods, Services and policies.
type reachCluster struct {
	t *testing.T
	m *inventory.Model
}

func newReachCluster(t *testing.T) *reachCluster {
	return &reachCluster{t: t, m: inventory.NewModel(inventory.Options{})}
}

func (c *reachCluster) load(schema kube.Schema, obj any) {
	c.t.Helper()
	at := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	for _, o := range kube.NewTranslator(schema).Added(obj, true, at) {
		_, err := c.m.Apply(o)
		require.NoError(c.t, err)
	}
}

func (c *reachCluster) pod(ns, name string, labels map[string]string) {
	c.load(kube.PodSchema{}, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: ns, Labels: labels}})
}

// dbService is Service db in ns selecting app=db pods, port 5432.
func (c *reachCluster) dbService(ns string) inventory.EntityID {
	c.load(kube.ServiceSchema{}, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: ns},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "db"},
			Ports: []corev1.ServicePort{{Port: 5432,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromInt32(5432)}},
		},
	})
	return inventory.CoreID(kube.KindService, ns, "db")
}

func (c *reachCluster) policy(
	ns string, spec networkingv1.NetworkPolicySpec,
) inventory.EntityID {
	name := "p" + string(rune('a'+len(c.m.Entities(kube.KindNetworkPolicy))))
	c.load(kube.NetworkPolicySchema{}, &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       spec})
	return inventory.CoreID(kube.KindNetworkPolicy, ns, name)
}

func (c *reachCluster) blocked(port int) (kube.Block, bool) {
	caller := inventory.CoreID(kube.KindPod, "shop", "api-0")
	return kube.CallBlocked(c.m, caller, inventory.CoreID(
		kube.KindService, "data", "db"), port)
}

// twoNamespaces has an api pod in shop and a db pod behind Service db in
// data.
func twoNamespaces(t *testing.T) *reachCluster {
	c := newReachCluster(t)
	c.pod("shop", "api-0", map[string]string{"app": "api"})
	c.pod("data", "db-0", map[string]string{"app": "db"})
	c.dbService("data")
	return c
}

var denyAllEgress = networkingv1.NetworkPolicySpec{
	PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
}

func TestCallBlockedOpenWithoutPolicies(t *testing.T) {
	_, blocked := twoNamespaces(t).blocked(5432)

	assert.False(t, blocked)
}

func TestCallBlockedByDefaultDenyEgress(t *testing.T) {
	c := twoNamespaces(t)
	policy := c.policy("shop", denyAllEgress)

	block, blocked := c.blocked(5432)

	require.True(t, blocked)
	assert.True(t, block.Egress)
	assert.Equal(t, []inventory.EntityID{policy}, block.Policies)
	assert.Equal(t, "api-0", block.Pod.Name)
}

func TestCallBlockedByDefaultDenyIngress(t *testing.T) {
	c := twoNamespaces(t)
	policy := c.policy("data", networkingv1.NetworkPolicySpec{})

	block, blocked := c.blocked(5432)

	require.True(t, blocked)
	assert.False(t, block.Egress)
	assert.Equal(t, []inventory.EntityID{policy}, block.Policies)
}

func TestCallBlockedLiftedByAllowingRule(t *testing.T) {
	c := twoNamespaces(t)
	spec := denyAllEgress
	spec.Egress = []networkingv1.NetworkPolicyEgressRule{{
		To: []networkingv1.NetworkPolicyPeer{{
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
				"kubernetes.io/metadata.name": "data"}},
			PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
				"app": "db"}},
		}},
	}}
	c.policy("shop", spec)

	_, blocked := c.blocked(5432)

	assert.False(t, blocked)
}

func TestCallBlockedWhenRuleNamesAnotherNamespace(t *testing.T) {
	c := twoNamespaces(t)
	spec := denyAllEgress
	spec.Egress = []networkingv1.NetworkPolicyEgressRule{{
		To: []networkingv1.NetworkPolicyPeer{{
			PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
				"app": "db"}}, // a pod selector alone means shop's pods
		}},
	}}
	c.policy("shop", spec)

	_, blocked := c.blocked(5432)

	assert.True(t, blocked)
}

func TestCallBlockedByPortMismatch(t *testing.T) {
	c := twoNamespaces(t)
	spec := denyAllEgress
	tcp := corev1.ProtocolTCP
	other := intstr.FromInt32(6379)
	spec.Egress = []networkingv1.NetworkPolicyEgressRule{{
		Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp,
			Port: &other}},
	}}
	c.policy("shop", spec)

	_, blocked := c.blocked(5432)

	assert.True(t, blocked)
}

func TestCallBlockedNamedPortIsNotShownDenied(t *testing.T) {
	c := twoNamespaces(t)
	spec := denyAllEgress
	named := intstr.FromString("postgres")
	spec.Egress = []networkingv1.NetworkPolicyEgressRule{{
		Ports: []networkingv1.NetworkPolicyPort{{Port: &named}},
	}}
	c.policy("shop", spec)

	_, blocked := c.blocked(5432)

	assert.False(t, blocked)
}

func TestCallBlockedIgnoresPoliciesOfOtherPods(t *testing.T) {
	c := twoNamespaces(t)
	spec := denyAllEgress
	spec.PodSelector = metav1.LabelSelector{MatchLabels: map[string]string{
		"app": "worker"}}
	c.policy("shop", spec)

	_, blocked := c.blocked(5432)

	assert.False(t, blocked)
}

func TestCallBlockedNeedsPodsBehindTheService(t *testing.T) {
	c := newReachCluster(t)
	c.pod("shop", "api-0", map[string]string{"app": "api"})
	c.dbService("data")
	c.policy("shop", denyAllEgress)

	_, blocked := c.blocked(5432)

	assert.False(t, blocked, "no pod behind the Service proves nothing")
}

func TestPolicyRulesFollowKubernetesDefaults(t *testing.T) {
	assert.Equal(t, kube.PolicyRules{Ingress: true}, rulesOf(
		networkingv1.NetworkPolicySpec{}))
	assert.Equal(t, kube.PolicyRules{Ingress: true, Egress: true,
		Out: []kube.PolicyRule{{}}}, rulesOf(networkingv1.NetworkPolicySpec{
		Egress: []networkingv1.NetworkPolicyEgressRule{{}}}))
	assert.Equal(t, kube.PolicyRules{Egress: true}, rulesOf(denyAllEgress))
}

func rulesOf(spec networkingv1.NetworkPolicySpec) kube.PolicyRules {
	d, _ := kube.NetworkPolicySchema{}.Describe(&networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec:       spec})
	rules, _ := kube.ParsePolicyRules(d.Attributes[kube.AttrPolicyRules].AsText())
	return rules
}
