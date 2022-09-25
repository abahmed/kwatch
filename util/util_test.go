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
	assert := assert.New(t)

	cli := fake.NewSimpleClientset()
	event := v1.Event{
		Reason:        "test reason",
		Message:       "test message",
		LastTimestamp: metav1.Now(),
	}
	cli.PrependReactor("list", "events", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, &v1.EventList{
			Items: []v1.Event{event},
		}, nil
	})

	result := GetPodEventsStr(cli, "dummy-app-579f7cd745-t6fdg", "test")
	expectedOutput :=
		"[" + event.LastTimestamp.String() + "] " + event.Reason + " " +
			event.Message
	assert.Equal(result, expectedOutput)

}
func TestGetPodEventsStrError(t *testing.T) {
	assert := assert.New(t)

	cli := fake.NewSimpleClientset()

	cli.PrependReactor("list", "events", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("ssss")
	})

	result := GetPodEventsStr(cli, "dummy-app-579f7cd745-t6fdg", "test")
	assert.Equal(result, "")
}

func TestContainsKillingStoppingContainerEvents(t *testing.T) {
	assert := assert.New(t)

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

	result :=
		ContainsKillingStoppingContainerEvents(
			cli,
			"dummy-app-579f7cd745-t6fdg",
			"test")

	assert.True(result)
}

func TestContainsKillingStoppingContainerEventsError(t *testing.T) {
	assert := assert.New(t)

	cli := fake.NewSimpleClientset()
	cli.PrependReactor(
		"list",
		"events",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("ssss")
		})

	result :=
		ContainsKillingStoppingContainerEvents(
			cli,
			"dummy-app-579f7cd745-t6fdg",
			"test")

	assert.False(result)
}

func TestContainsKillingStoppingContainerEmpty(t *testing.T) {
	assert := assert.New(t)

	cli := fake.NewSimpleClientset()
	cli.PrependReactor(
		"list",
		"events",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &v1.EventList{
				Items: []v1.Event{},
			}, nil
		})

	result :=
		ContainsKillingStoppingContainerEvents(
			cli,
			"dummy-app-579f7cd745-t6fdg",
			"test")

	assert.False(result)
}
