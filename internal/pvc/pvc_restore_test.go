package pvc

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestStartFailsClosedOnCorruptPersistedUsage(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kwatch-pvc", Namespace: "kwatch",
		},
		Data: map[string]string{"pvc-usage": "{"},
	})
	monitor := newTestPvcMonitorWithState(
		client, &config.PvcMonitor{Enabled: true}, nil,
		newTestPersistenceManager(client, "kwatch"),
	)
	if err := monitor.Start(context.Background()); err == nil {
		t.Fatal("corrupt persisted PVC usage did not block startup")
	}
}
