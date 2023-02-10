package util

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/client-go/kubernetes/fake"

	k8stesting "k8s.io/client-go/testing"
)

func TestGetPodContainerLogs(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	logs := GetPodContainerLogs(
		client,
		"test",
		"test",
		"default",
		false,
		20)

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
	cli.PrependReactor(
		"list",
		"events",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
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

	cli.PrependReactor(
		"list",
		"events",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
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

func TestRandomString(t *testing.T) {
	assert := assert.New(t)

	randLen := rand.Intn(300)
	result := RandomString(randLen)

	assert.Len(result, randLen)
}
