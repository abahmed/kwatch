package util

import (
	"testing"

	"github.com/abahmed/kwatch/event"
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
	logs := GetPodContainerLogs(client, podName, containerName, "default", false)
	if logs != "fake logs" {
		t.Fatalf(
			"get logs for %s in %s: returned %s expected %s",
			containerName,
			podName,
			logs,
			"")
	}
}

func TestIsListAllBool(t *testing.T) {
	testCases := []struct {
		boolean bool
		list    []bool
		output  bool
	}{
		{
			boolean: true,
			list:    []bool{true, true},
			output:  true,
		},
		{
			boolean: false,
			list:    []bool{false, false},
			output:  true,
		},
		{
			boolean: true,
			list:    []bool{false, true},
			output:  false,
		},
		{
			boolean: true,
			list:    []bool{},
			output:  true,
		},
	}

	for _, tc := range testCases {
		out := IsListAllBool(tc.boolean, tc.list)
		if out != tc.output {
			t.Fatalf(
				"IsListAllBool %t in %v: returned %t expected %t",
				tc.boolean,
				tc.list,
				out,
				tc.output)
		}
	}
}

func TestGetProviders(t *testing.T) {
	alertMap := map[string]interface{}{
		"slack": map[string]interface{}{
			"webhook": "test",
		},
		"pagerduty": map[string]interface{}{
			"integrationkey": "test",
		},
		"discord": map[string]interface{}{
			"webhook": "test",
		},
		"telegram": map[string]interface{}{
			"token":  "test",
			"chatid": "test",
		},
		"teams": map[string]interface{}{
			"webhook": "test",
		},
	}
	viper.SetDefault("alert", alertMap)
	providers := GetProviders()
	if len(providers) != len(alertMap) {
		t.Fatalf(
			"get providers returned %d expected %d",
			len(providers),
			len(alertMap))
	}
}

func TestSendProvidersEvent(t *testing.T) {
	alertMap := map[string]interface{}{
		"slack": map[string]interface{}{
			"webhook": "test",
		},
		"pagerduty": map[string]interface{}{
			"integrationkey": "test",
		},
		"discord": map[string]interface{}{
			"webhook": "test",
		},
		"telegram": map[string]interface{}{
			"token":  "test",
			"chatid": "test",
		},
		"teams": map[string]interface{}{
			"webhook": "test",
		},
	}
	viper.SetDefault("alert", alertMap)
	providers := GetProviders()

	SendProvidersEvent(providers, event.Event{})
}

func TestSendProvidersMsg(t *testing.T) {
	alertMap := map[string]interface{}{
		"slack": map[string]interface{}{
			"webhook": "test",
		},
		"pagerduty": map[string]interface{}{
			"integrationkey": "test",
		},
		"discord": map[string]interface{}{
			"webhook": "test",
		},
		"telegram": map[string]interface{}{
			"token":  "test",
			"chatid": "test",
		},
		"teams": map[string]interface{}{
			"webhook": "test",
		},
	}
	viper.SetDefault("alert", alertMap)
	providers := GetProviders()

	SendProvidersMsg(providers, "hello world!")
}
