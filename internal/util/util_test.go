package util

import (
	"errors"
	"math/rand"
	"os"
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
			Output: "te\\bst",
		},
		{
			Input:  "\b",
			Output: "\\b",
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

	event := v1.Event{
		Reason:        "test reason",
		Message:       "test message",
		LastTimestamp: metav1.Now(),
	}

	result := GetPodEventsStr(&[]v1.Event{event})
	expectedOutput :=
		"[" + event.LastTimestamp.String() + "] " + event.Reason + " " +
			event.Message
	assert.Equal(result, expectedOutput)
}

func TestGetPodEventsStrNil(t *testing.T) {
	assert := assert.New(t)

	result := GetPodEventsStr(nil)
	expectedOutput := ""
	assert.Equal(result, expectedOutput)
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

func TestGetNodes(t *testing.T) {
	assert := assert.New(t)

	cli := fake.NewSimpleClientset()
	node := v1.Node{}
	cli.PrependReactor(
		"list",
		"nodes",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &v1.NodeList{
				Items: []v1.Node{node},
			}, nil
		})

	result, err := GetNodes(cli)
	assert.NoError(err)
	assert.NotNil(result)
	assert.Equal(len(result.Items), 1)
}

func TestGetPVNameFromPVC(t *testing.T) {
	assert := assert.New(t)

	cli := fake.NewSimpleClientset()
	cli.PrependReactor(
		"get",
		"persistentvolumeclaims",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					VolumeName: "test",
				},
			}, nil
		})

	result, err := GetPVNameFromPVC(cli, "test", "test")
	assert.NoError(err)
	assert.Equal(result, "test")
}

func TestGetPVNameFromPVCError(t *testing.T) {
	assert := assert.New(t)

	cli := fake.NewSimpleClientset()
	cli.PrependReactor(
		"get",
		"persistentvolumeclaims",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("failed")
		})

	result, err := GetPVNameFromPVC(cli, "test", "test")
	assert.Error(err, "failed")
	assert.Equal(result, "")
}

func TestGetPVNameFromPVCEmpty(t *testing.T) {
	assert := assert.New(t)

	cli := fake.NewSimpleClientset()
	cli.PrependReactor(
		"get",
		"persistentvolumeclaims",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					VolumeName: "",
				},
			}, nil
		})

	result, err := GetPVNameFromPVC(cli, "test", "test")
	assert.NoError(err)
	assert.Equal("", result)
}

func TestGetNodesEmpty(t *testing.T) {
	assert := assert.New(t)

	cli := fake.NewSimpleClientset()
	cli.PrependReactor(
		"list",
		"nodes",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &v1.NodeList{
				Items: []v1.Node{},
			}, nil
		})

	result, err := GetNodes(cli)
	assert.NoError(err)
	assert.NotNil(result)
	assert.Equal(0, len(result.Items))
}

func TestGetPodEventsStrMultipleEvents(t *testing.T) {
	assert := assert.New(t)

	events := []v1.Event{
		{
			Reason:        "Started",
			Message:       "Container started",
			LastTimestamp: metav1.Now(),
		},
		{
			Reason:        "Killed",
			Message:       "Container killed",
			LastTimestamp: metav1.Now(),
		},
	}

	result := GetPodEventsStr(&events)
	assert.NotEmpty(result)
	assert.Contains(result, "Started")
	assert.Contains(result, "Container started")
	assert.Contains(result, "Killed")
	assert.Contains(result, "Container killed")
}

func TestContainsKillingStoppingContainerDifferentCase(t *testing.T) {
	assert := assert.New(t)

	cli := fake.NewSimpleClientset()
	cli.PrependReactor(
		"list",
		"events",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &v1.EventList{
				Items: []v1.Event{{
					Reason:        "KILLING",
					Message:       "Stopping Container",
					LastTimestamp: metav1.Now(),
				}},
			}, nil
		})

	result := ContainsKillingStoppingContainerEvents(cli, "test", "default")
	assert.True(result)
}

func TestContainsKillingStoppingContainerNoMatch(t *testing.T) {
	assert := assert.New(t)

	cli := fake.NewSimpleClientset()
	cli.PrependReactor(
		"list",
		"events",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &v1.EventList{
				Items: []v1.Event{{
					Reason:        "Started",
					Message:       "Container started normally",
					LastTimestamp: metav1.Now(),
				}},
			}, nil
		})

	result := ContainsKillingStoppingContainerEvents(cli, "test", "default")
	assert.False(result)
}

func TestRandomStringZero(t *testing.T) {
	assert := assert.New(t)

	result := RandomString(0)
	assert.Equal("", result)
}

func TestRandomStringLength(t *testing.T) {
	assert := assert.New(t)

	testLengths := []int{1, 5, 10, 50, 100}
	for _, length := range testLengths {
		result := RandomString(length)
		assert.Len(result, length)
	}
}

func TestRandomStringUniqueness(t *testing.T) {
	assert := assert.New(t)

	results := make(map[string]bool)
	for i := 0; i < 100; i++ {
		result := RandomString(50)
		results[result] = true
	}
	assert.Equal(100, len(results))
}

func TestJsonEscapeEmpty(t *testing.T) {
	assert := assert.New(t)

	result := JsonEscape("")
	assert.Equal("", result)
}

func TestJsonEscapeMultipleSpecialChars(t *testing.T) {
	assert := assert.New(t)

	result := JsonEscape("test\"with\\special\nchars")
	assert.NotEqual("test\"with\\special\nchars", result)
}

func TestSetLogFormatterDefault(t *testing.T) {
	assert := assert.New(t)

	SetLogFormatter("text")
	assert.NotNil(t)
}

func TestSetLogFormatterJSON(t *testing.T) {
	assert := assert.New(t)

	SetLogFormatter("json")
	assert.NotNil(t)
}

func TestSetLogFormatterUnknown(t *testing.T) {
	assert := assert.New(t)

	SetLogFormatter("unknown")
	assert.NotNil(t)
}

func TestGetPodEventsWithFieldSelector(t *testing.T) {
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

	result, err := GetPodEvents(cli, "my-pod", "test-namespace")
	assert.NoError(err)
	assert.NotNil(result)
	assert.Equal(0, len(result.Items))
}

func TestGetNamespaceFromEnv(t *testing.T) {
	assert := assert.New(t)

	os.Setenv("POD_NAMESPACE", "custom-namespace")
	defer os.Unsetenv("POD_NAMESPACE")

	result := GetNamespace()
	assert.Equal("custom-namespace", result)
}

func TestGetNamespaceDefault(t *testing.T) {
	assert := assert.New(t)

	os.Unsetenv("POD_NAMESPACE")

	result := GetNamespace()
	assert.Equal("kwatch", result)
}

func TestGetPodEventsSuccess(t *testing.T) {
	assert := assert.New(t)

	cli := fake.NewSimpleClientset()
	cli.PrependReactor(
		"list",
		"events",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &v1.EventList{
				Items: []v1.Event{
					{
						Reason:  "Normal",
						Message: "Test event",
					},
				},
			}, nil
		})

	result, err := GetPodEvents(cli, "test-pod", "default")
	assert.NoError(err)
	assert.NotNil(result)
	assert.Equal(1, len(result.Items))
}
