package util

import (
	"errors"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/client-go/kubernetes/fake"

	k8stesting "k8s.io/client-go/testing"
)

func TestIsStrInSlice(t *testing.T) {
	assert := assert.New(t)

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
		assert.Equal(out, tc.output)
	}
}

func TestGetPodContainerLogs(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	viper.SetDefault("maxRecentLogLines", 20)
	podName := "test"
	containerName := "test"
	logs := GetPodContainerLogs(
		client,
		podName,
		containerName,
		"default",
		false)
	assert.Equal(logs, "fake logs")
}

func TestJsonEscape(t *testing.T) {
	assert := assert.New(t)

	testCases := []struct {
		Input  string
		Output string
	}{
		{
			Input:  "test",
			Output: "test",
		},
		{
			Input:  "te\bst",
			Output: "te\\u0008st",
		},
		{
			Input:  "\b",
			Output: "\\u0008",
		},
		{
			Input:  "\"",
			Output: "\\\"",
		},
	}

	for _, tc := range testCases {
		assert.Equal(JsonEscape(tc.Input), tc.Output)
	}
}

func TestGetPodEventsStr(t *testing.T) {
	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dummy-app-579f7cd745-t6fdg",
			Namespace: "test",
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	cli := fake.NewSimpleClientset(pod)

	cli.PrependReactor("list", "events", func(action k8stesting.Action) (bool, runtime.Object, error) {
		events := []v1.Event{{
			Reason:        "test reason",
			Message:       "test message",
			LastTimestamp: metav1.Now(),
		}}
		return true, &v1.EventList{
			Items: events,
		}, nil
	})

	GetPodEventsStr(cli, "dummy-app-579f7cd745-t6fdg", "test")
}
func TestGetPodEventsStrError(t *testing.T) {
	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dummy-app-579f7cd745-t6fdg",
			Namespace: "test",
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	cli := fake.NewSimpleClientset(pod)

	cli.PrependReactor("list", "events", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("ssss")
	})

	GetPodEventsStr(cli, "dummy-app-579f7cd745-t6fdg", "test")
}

func TestContainsKillingStoppingContainerEvents(t *testing.T) {
	cli := fake.NewSimpleClientset()
	cli.PrependReactor(
		"list",
		"events",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &v1.EventList{
				Items: []v1.Event{{
					Reason:        "killing",
					Message:       "test stopping container",
					LastTimestamp: metav1.Now(),
				}},
			}, nil
		})

	ContainsKillingStoppingContainerEvents(cli, "dummy-app-579f7cd745-t6fdg", "test")
}
