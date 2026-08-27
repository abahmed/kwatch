package k8s

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

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
		context.Background(),
		client,
		"test",
		"test",
		"default",
		false,
		20)

	assert.Equal(logs, "fake logs")
}

func TestGetPodContainerLogsError(t *testing.T) {
	assert := assert.New(t)

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
		"test-pod",
		"test-container",
		"default",
		false,
		20)

	assert.Equal(
		"",
		logs,
		"GetPodContainerLogs should return empty string on error",
	)
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
		event.LastTimestamp.UTC().Format("Jan 02 15:04:05") +
			"  " + event.Reason + "  " +
			event.Message
	assert.Equal(result, expectedOutput)
}

func TestGetPodEventsStrFallsBackToFirstTimestamp(t *testing.T) {
	assert := assert.New(t)

	ts := metav1.Now()
	event := v1.Event{
		Reason:         "Started",
		Message:        "Container started",
		FirstTimestamp: ts,
	}

	result := GetPodEventsStr(&[]v1.Event{event})
	expectedOutput :=
		ts.Time.UTC().Format("Jan 02 15:04:05") + "  Started  Container started"
	assert.Equal(expectedOutput, result)
}

func TestGetPodEventsStrFallsBackToCreationTimestamp(t *testing.T) {
	assert := assert.New(t)

	ts := metav1.Now()
	event := v1.Event{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: ts},
		Reason:     "Pulled",
		Message:    "Pulled image",
	}

	result := GetPodEventsStr(&[]v1.Event{event})
	expectedOutput :=
		ts.Time.UTC().Format("Jan 02 15:04:05") + "  Pulled  Pulled image"
	assert.Equal(expectedOutput, result)
}

func TestGetPodEventsStrZeroTimestampOmitted(t *testing.T) {
	assert := assert.New(t)

	// No timestamp fields set at all: must not render the zero
	// timestamp (0001-01-01 ...) into the notification.
	event := v1.Event{
		Reason:  "Scheduled",
		Message: "Successfully assigned pod",
	}

	result := GetPodEventsStr(&[]v1.Event{event})
	assert.Equal("Scheduled Successfully assigned pod", result)
	assert.NotContains(result, "0001-01-01")
}

func TestGetPodEventsStrNil(t *testing.T) {
	assert := assert.New(t)

	result := GetPodEventsStr(nil)
	expectedOutput := ""
	assert.Equal(result, expectedOutput)
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

	result, err := GetNodes(context.Background(), cli)
	assert.NoError(err)
	assert.NotNil(result)
	assert.Equal(len(result.Items), 1)
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

	result, err := GetNodes(context.Background(), cli)
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

	result, err := GetPodEvents(
		context.Background(),
		cli,
		"my-pod",
		"test-namespace",
	)
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

	result, err := GetPodEvents(
		context.Background(),
		cli,
		"test-pod",
		"default",
	)
	assert.NoError(err)
	assert.NotNil(result)
	assert.Equal(1, len(result.Items))
}

func TestGetDefaultClient(t *testing.T) {
	assert := assert.New(t)

	client := GetDefaultClient()
	assert.NotNil(client)
	assert.Equal(DefaultHTTPTimeout, client.Timeout)
}

// Events are unbounded in Kubernetes; a churning pod accumulates hundreds. An
// unbounded list pushed alerts past the provider's limits and lost the whole
// alert. The cap keeps the newest, which describe the current state.
func TestGetPodEventsStrKeepsNewestUpToCap(t *testing.T) {
	events := make([]v1.Event, 0, 300)
	for i := 299; i >= 0; i-- { // deliberately newest-first input
		events = append(events, v1.Event{
			Reason:        fmt.Sprintf("Event%03d", i),
			Message:       "m",
			LastTimestamp: metav1.NewTime(time.Unix(int64(1700000000+i*60), 0)),
		})
	}
	out := GetPodEventsStr(&events)
	lines := strings.Split(out, "\n")
	assert.Len(t, lines, MaxEventsInMessage+1, "cap plus the omission note")
	assert.Contains(t, lines[0], "260 earlier event(s) omitted")
	assert.Contains(t, out, "Event299", "the newest event survives")
	assert.NotContains(t, out, "Event000", "the oldest is shed first")
	assert.True(
		t,
		strings.Index(out, "Event260") < strings.Index(out, "Event299"),
		"oldest kept comes first",
	)
}
