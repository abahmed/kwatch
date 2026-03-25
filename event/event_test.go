package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEventStruct(t *testing.T) {
	assert := assert.New(t)

	e := Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		NodeName:      "node-1",
		Reason:        "OOMKILLED",
		Events:        "test events",
		Logs:          "test logs",
		Labels:        map[string]string{"app": "test"},
	}

	assert.Equal("test-pod", e.PodName)
	assert.Equal("test-container", e.ContainerName)
	assert.Equal("default", e.Namespace)
	assert.Equal("node-1", e.NodeName)
	assert.Equal("OOMKILLED", e.Reason)
	assert.Equal("test events", e.Events)
	assert.Equal("test logs", e.Logs)
	assert.Equal("test", e.Labels["app"])
}

func TestFormatMarkdown(t *testing.T) {
	assert := assert.New(t)

	e := Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		NodeName:      "node-1",
		Reason:        "OOMKILLED",
		Events:        "test events",
		Logs:          "test logs",
	}

	result := e.FormatMarkdown("test-cluster", "", "")
	assert.Contains(result, "test-cluster")
	assert.Contains(result, "test-pod")
	assert.Contains(result, "test-container")
	assert.Contains(result, "default")
	assert.Contains(result, "node-1")
	assert.Contains(result, "OOMKILLED")
	assert.Contains(result, "test events")
	assert.Contains(result, "test logs")
}

func TestFormatMarkdownWithCustomText(t *testing.T) {
	assert := assert.New(t)

	e := Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		NodeName:      "node-1",
		Reason:        "OOMKILLED",
		Events:        "test events",
		Logs:          "test logs",
	}

	result := e.FormatMarkdown("test-cluster", "Custom alert message", "")
	assert.Contains(result, "Custom alert message")
}

func TestFormatMarkdownWithCustomDelimiter(t *testing.T) {
	assert := assert.New(t)

	e := Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		NodeName:      "node-1",
		Reason:        "OOMKILLED",
		Events:        "test events",
		Logs:          "test logs",
	}

	result := e.FormatMarkdown("test-cluster", "", "\n\n")
	assert.Contains(result, "test-cluster")
}

func TestFormatMarkdownEmptyEventsLogs(t *testing.T) {
	assert := assert.New(t)

	e := Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		NodeName:      "node-1",
		Reason:        "OOMKILLED",
		Events:        "",
		Logs:          "",
	}

	result := e.FormatMarkdown("test-cluster", "", "")
	assert.Contains(result, "test-cluster")
	assert.Contains(result, "test-pod")
}

func TestFormatHtml(t *testing.T) {
	assert := assert.New(t)

	e := Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		NodeName:      "node-1",
		Reason:        "OOMKILLED",
		Events:        "test events",
		Logs:          "test logs",
	}

	result := e.FormatHtml("test-cluster", "")
	assert.Contains(result, "test-cluster")
	assert.Contains(result, "test-pod")
	assert.Contains(result, "test-container")
	assert.Contains(result, "default")
	assert.Contains(result, "node-1")
	assert.Contains(result, "OOMKILLED")
	assert.Contains(result, "<b>Events:</b>")
	assert.Contains(result, "<b>Logs:</b>")
}

func TestFormatHtmlWithCustomText(t *testing.T) {
	assert := assert.New(t)

	e := Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		NodeName:      "node-1",
		Reason:        "OOMKILLED",
		Events:        "test events",
		Logs:          "test logs",
	}

	result := e.FormatHtml("test-cluster", "Custom HTML alert")
	assert.Contains(result, "Custom HTML alert")
}

func TestFormatHtmlEmptyEventsLogs(t *testing.T) {
	assert := assert.New(t)

	e := Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		NodeName:      "node-1",
		Reason:        "OOMKILLED",
		Events:        "",
		Logs:          "",
	}

	result := e.FormatHtml("test-cluster", "")
	assert.Contains(result, "test-cluster")
}

func TestFormatText(t *testing.T) {
	assert := assert.New(t)

	e := Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		NodeName:      "node-1",
		Reason:        "OOMKILLED",
		Events:        "test events",
		Logs:          "test logs",
	}

	result := e.FormatText("test-cluster", "")
	assert.Contains(result, "test-cluster")
	assert.Contains(result, "test-pod")
	assert.Contains(result, "test-container")
	assert.Contains(result, "default")
	assert.Contains(result, "node-1")
	assert.Contains(result, "OOMKILLED")
	assert.Contains(result, "Events:")
	assert.Contains(result, "Logs:")
}

func TestFormatTextWithCustomText(t *testing.T) {
	assert := assert.New(t)

	e := Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		NodeName:      "node-1",
		Reason:        "OOMKILLED",
		Events:        "test events",
		Logs:          "test logs",
	}

	result := e.FormatText("test-cluster", "Custom text alert")
	assert.Contains(result, "There is an issue with container in a pod!")
}

func TestFormatTextEmptyEventsLogs(t *testing.T) {
	assert := assert.New(t)

	e := Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		NodeName:      "node-1",
		Reason:        "OOMKILLED",
		Events:        "",
		Logs:          "",
	}

	result := e.FormatText("test-cluster", "")
	assert.Contains(result, "test-cluster")
}

func TestFormatTextOnlyWhitespaceEventsLogs(t *testing.T) {
	assert := assert.New(t)

	e := Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		NodeName:      "node-1",
		Reason:        "OOMKILLED",
		Events:        "   \n   ",
		Logs:          "   \n   ",
	}

	result := e.FormatText("test-cluster", "")
	assert.Contains(result, "test-cluster")
}
