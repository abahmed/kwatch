package controller

import (
	"testing"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/graphcontext"
)

func TestWebhookGraphRebuildsAndDeduplicatesServices(t *testing.T) {
	graph := graphcontext.NewResourceGraph()
	ctrl := &Controller{graphRuntime: graphRuntime{graph: graph}}
	service := func(namespace, name string) *admissionv1.ServiceReference {
		return &admissionv1.ServiceReference{
			Namespace: namespace, Name: name,
		}
	}
	ctrl.rebuildMutatingWebhookGraph(&admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "mutating"},
		Webhooks: []admissionv1.MutatingWebhook{
			{ClientConfig: admissionv1.WebhookClientConfig{
				Service: service("ns", "api"),
			}},
			{ClientConfig: admissionv1.WebhookClientConfig{
				Service: service("ns", "api"),
			}},
			{ClientConfig: admissionv1.WebhookClientConfig{
				Service: service("", "ignored"),
			}},
		},
	})
	ctrl.rebuildValidatingWebhookGraph(&admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "validating"},
		Webhooks: []admissionv1.ValidatingWebhook{
			{ClientConfig: admissionv1.WebhookClientConfig{
				Service: service("ns", "api"),
			}},
		},
	})

	if got := graph.DependenciesOf(
		"mutatingwebhookconfiguration", "", "mutating",
	); len(got) != 1 {
		t.Fatalf("mutating dependencies = %#v, want one", got)
	}
	if got := graph.DependenciesOf(
		"validatingwebhookconfiguration", "", "validating",
	); len(got) != 1 {
		t.Fatalf("validating dependencies = %#v, want one", got)
	}
}

func TestWebhookGraphRebuildIgnoresInvalidInputs(t *testing.T) {
	ctrl := &Controller{}
	ctrl.rebuildMutatingWebhookGraph("not a webhook")
	ctrl.rebuildValidatingWebhookGraph(nil)

	graph := graphcontext.NewResourceGraph()
	ctrl.graph = graph
	ctrl.replaceWebhookEdges(
		"mutatingwebhookconfiguration", "empty",
		[]*admissionv1.ServiceReference{
			nil, {Name: "missing-namespace"}, {Namespace: "ns"},
		},
	)
	if got := graph.Edges(); len(got) != 0 {
		t.Fatalf("invalid webhook edges = %#v", got)
	}
}
