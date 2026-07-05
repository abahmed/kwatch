package handler

import (
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestProcessNetworkPolicyCreatesIncident(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetNetpolLister(f.Networking().V1().NetworkPolicies().Lister())

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-netpol", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		},
	}
	f.Networking().V1().NetworkPolicies().Informer().GetIndexer().Add(policy)

	assert.NoError(t, h.ProcessNetworkPolicy("default/test-netpol", false))

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "RestrictiveNetworkPolicy" && v.State == model.StateActive {
			found = true
		}
	}
	assert.True(t, found, "key-based ProcessNetworkPolicy should create incident")
}

func TestProcessNetworkPolicyKeyDeleted(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessNetworkPolicy("default/test-netpol", true))
}

func TestProcessNetworkPolicyKeyNotFound(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetNetpolLister(f.Networking().V1().NetworkPolicies().Lister())

	assert.NoError(t, h.ProcessNetworkPolicy("default/missing", false))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestDetectNetworkPolicyNoEgressPolicyType(t *testing.T) {
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-netpol", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		},
	}
	sig := DetectNetworkPolicyIssue(policy)
	assert.Nil(t, sig, "policy without egress type should not produce a signal")
}

func TestDetectNetworkPolicyDenyAllEgress(t *testing.T) {
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-netpol", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		},
	}
	sig := DetectNetworkPolicyIssue(policy)
	assert.NotNil(t, sig)
	assert.Equal(t, "RestrictiveNetworkPolicy", sig.Reason)
	assert.Equal(t, "networkpolicy", sig.Resource)
	assert.Equal(t, "default/test-netpol", sig.Owner)
}

func TestDetectNetworkPolicyDenyAllEgressEmptyPolicyTypes(t *testing.T) {
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-netpol", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{},
		},
	}
	sig := DetectNetworkPolicyIssue(policy)
	assert.NotNil(t, sig, "empty policy types with no egress rules should be flagged")
}

func TestDetectNetworkPolicyHasEgressRules(t *testing.T) {
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-netpol", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{Port: &intstr.IntOrString{Type: intstr.Int, IntVal: 443}},
					},
				},
			},
		},
	}
	sig := DetectNetworkPolicyIssue(policy)
	assert.Nil(t, sig, "policy with egress rules should not produce a signal")
}

func TestDetectNetworkPolicyEmptyPolicyTypesWithEgressRules(t *testing.T) {
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-netpol", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{Port: &intstr.IntOrString{Type: intstr.Int, IntVal: 443}},
					},
				},
			},
		},
	}
	sig := DetectNetworkPolicyIssue(policy)
	assert.Nil(t, sig, "empty policy types with egress rules should not produce a signal")
}

func TestDetectNetworkPolicyEgressAndIngressWithNoEgressRules(t *testing.T) {
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-netpol", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		},
	}
	sig := DetectNetworkPolicyIssue(policy)
	assert.NotNil(t, sig, "policy with egress type but no egress rules should be flagged")
}

func TestProcessNetworkPolicyObjectCreatesAndResolves(t *testing.T) {
	var mu sync.Mutex
	var resolves int

	e := correlation.NewEngine(correlation.Config{
		Window: 10 * time.Minute,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			mu.Lock()
			defer mu.Unlock()
			if action == model.ActionResolved {
				resolves++
			}
		},
	})

	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-netpol", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		},
	}

	// Deny-all egress should create incident
	assert.NoError(t, h.ProcessNetworkPolicyObject(policy, false))
	assert.Equal(t, 1, e.ActiveCount())

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "RestrictiveNetworkPolicy" {
			found = true
		}
	}
	assert.True(t, found, "RestrictiveNetworkPolicy incident should exist")

	// Add egress rules to resolve
	policy.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{
		{
			Ports: []networkingv1.NetworkPolicyPort{
				{Port: &intstr.IntOrString{Type: intstr.Int, IntVal: 443}},
			},
		},
	}

	assert.NoError(t, h.ProcessNetworkPolicyObject(policy, false))

	mu.Lock()
	r := resolves
	mu.Unlock()
	assert.Equal(t, 1, r, "RestrictiveNetworkPolicy should resolve when egress rules are added")
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessNetworkPolicyObjectNoIssue(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-netpol", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		},
	}

	assert.NoError(t, h.ProcessNetworkPolicyObject(policy, false))
	assert.Equal(t, 0, e.ActiveCount(), "ingress-only policy should not create incident")
}

func TestProcessNetworkPolicyObjectDeleted(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessNetworkPolicy("default/test-netpol", true))
}

func TestProcessNetworkPolicyObjectNil(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessNetworkPolicyObject(nil, false))
}
