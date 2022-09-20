package util

import (
	"testing"

	"github.com/spf13/viper"
	testclient "k8s.io/client-go/kubernetes/fake"
)

func TestIsStrInSlice(t *testing.T) {
	testCases := []struct {
		str    string
		list   []string
		output bool
	}{
		{
			str:    "hello",
			list:   []string{"hello", "world"},
			output: true,
		},
		{
			str:    "test",
			list:   []string{"hello", "world"},
			output: false,
		},
		{
			str:    "hello",
			list:   []string{},
			output: false,
		},
	}

	for _, tc := range testCases {
		out := IsStrInSlice(tc.str, tc.list)
		if out != tc.output {
			t.Fatalf(
				"search %s in %v: returned %t expected %t",
				tc.str,
				tc.list,
				out,
				tc.output)
		}
	}
}

func TestGetPodEventsStr(t *testing.T) {
	client := testclient.NewSimpleClientset()

	podName := "test-pod"
	events := GetPodEventsStr(client, podName, "default")
	if len(events) > 0 {
		t.Fatalf(
			"get events for %s: returned %s expected %s",
			podName,
			events,
			"")
	}
}

func TestGetPodContainerLogs(t *testing.T) {
	client := testclient.NewSimpleClientset()
	viper.SetDefault("maxRecentLogLines", 20)
	podName := "test"
	containerName := "test"
	logs := GetPodContainerLogs(
		client,
		podName,
		containerName,
		"default",
		false)
	if logs != "fake logs" {
		t.Fatalf(
			"get logs for %s in %s: returned %s expected %s",
			containerName,
			podName,
			logs,
			"")
	}
}
