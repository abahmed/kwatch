package kubelet

import (
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestGetPodContainerLogsReturnsContainerOutput(t *testing.T) {
	client := fake.NewSimpleClientset()

	logs := GetPodContainerLogs(
		context.Background(), client, "pod", "container", "default", false, 20,
	)

	if logs != "fake logs" {
		t.Fatalf("logs = %q, want %q", logs, "fake logs")
	}
}

func TestGetPodContainerLogsReturnsEmptyOnAPIError(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor(
		"get",
		"pods",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			if action.GetSubresource() == "log" {
				return true, nil, errors.New("log fetch error")
			}
			return false, nil, nil
		},
	)

	logs := GetPodContainerLogs(
		context.Background(),
		client,
		"pod",
		"container",
		"default",
		false,
		20,
	)

	if logs != "" {
		t.Fatalf("logs = %q, want empty output", logs)
	}
}
