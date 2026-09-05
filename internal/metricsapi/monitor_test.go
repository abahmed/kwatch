package metricsapi

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestListPodsUsesOneBulkList(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: "one", Namespace: "apps",
		}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: "two", Namespace: "apps",
		}},
	)
	monitor := &Monitor{client: client}

	pods, err := monitor.listPods(context.Background(), "apps")
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 2 {
		t.Fatalf("unexpected pod count: %d", len(pods))
	}
	actions := client.Actions()
	if len(actions) != 1 || actions[0].GetVerb() != "list" {
		t.Fatalf("expected one bulk list, got %#v", actions)
	}
}
