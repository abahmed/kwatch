package network

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

func TestNetworkRuntimeReconcilesRestrictivePolicy(t *testing.T) {
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "deny-egress",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
		},
	}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	if err := indexer.Add(policy); err != nil {
		t.Fatal(err)
	}
	sink := &networkSinkRecorder{}
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	runtime.ConfigureSources(Sources{
		NetworkPolicy: networkingv1lister.NewNetworkPolicyLister(indexer),
	})

	if err := runtime.ProcessNetworkPolicy(
		"default/deny-egress", false,
	); err != nil {
		t.Fatalf("ProcessNetworkPolicy() returned error: %v", err)
	}
	if len(sink.findings) != 1 ||
		sink.findings[0].Reason != constant.ReasonRestrictiveNetworkPolicy {
		t.Fatalf("unexpected findings: %#v", sink.findings)
	}
}

func TestNetworkRuntimeSkipsUnavailableListers(t *testing.T) {
	sink := &networkSinkRecorder{}
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)

	if err := runtime.ProcessNetworkPolicy("default/policy", false); err != nil {
		t.Fatalf("ProcessNetworkPolicy() returned error: %v", err)
	}
	if err := runtime.ProcessIngress("default/ingress", false); err != nil {
		t.Fatalf("ProcessIngress() returned error: %v", err)
	}
	if sink.gone != 0 {
		t.Fatalf("unavailable listers resolved %d subjects", sink.gone)
	}
}

func TestServiceObjectSkipsDeletedWhenEndpointListerIsUnavailable(
	t *testing.T,
) {
	sink := &networkSinkRecorder{}
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api"},
	}

	if err := runtime.ProcessServiceObject(service, true); err != nil {
		t.Fatalf("ProcessServiceObject() returned error: %v", err)
	}
	if sink.gone != 0 {
		t.Fatal("unavailable EndpointSlice lister resolved a service")
	}
}

type networkSinkRecorder struct {
	findings []*model.Observation
	gone     int
}

func (r *networkSinkRecorder) Process(
	finding *model.Observation,
) (*model.Incident, model.IncidentAction) {
	r.findings = append(r.findings, finding)
	return nil, model.ActionSkip
}

func (r *networkSinkRecorder) Resolve(model.ObjectRef, string) {}

func (r *networkSinkRecorder) ResolveObserved(*model.Observation) {}

func (r *networkSinkRecorder) Reconcile(
	_ model.ObjectRef, findings []*model.Observation,
) {
	for _, finding := range findings {
		if finding != nil {
			r.findings = append(r.findings, finding)
		}
	}
}

func (r *networkSinkRecorder) ReconcileGone(model.ObjectRef) { r.gone++ }
