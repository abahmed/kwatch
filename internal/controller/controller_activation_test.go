package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestControllerConstructionDoesNotStartInformers(t *testing.T) {
	client := fake.NewSimpleClientset()
	controller, cleanup := newTestController(
		t, client, &config.Config{}, &mockHandler{},
	)
	defer cleanup()

	_, err := client.CoreV1().Pods("default").Create(
		context.Background(),
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "standby-pod", Namespace: "default",
			},
		},
		metav1.CreateOptions{},
	)
	require.NoError(t, err)
	_, err = controller.PodLister().Pods("default").Get("standby-pod")
	require.Error(t, err)
}
