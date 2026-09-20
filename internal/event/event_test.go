package event

import (
	"io"
	"net/http"
	"strings"
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
		IncludeEvents: true,
		IncludeLogs:   true,
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
		IncludeEvents: true,
		IncludeLogs:   true,
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
		IncludeEvents: true,
		IncludeLogs:   true,
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
		IncludeEvents: true,
		IncludeLogs:   true,
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
		IncludeEvents: true,
		IncludeLogs:   true,
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

func TestFormatHtmlEscapesEventData(t *testing.T) {
	e := Event{
		PodName:       "pod<&",
		Reason:        "<script>alert(1)</script>",
		Events:        "<event>\n& details",
		Logs:          "<log>\n\"quoted\"",
		IncludeEvents: true,
		IncludeLogs:   true,
	}

	result := e.FormatHtml("cluster<&", "<custom>")
	assert.NotContains(t, result, "<script>")
	assert.NotContains(t, result, "<event>")
	assert.NotContains(t, result, "<log>")
	assert.Contains(t, result, "&lt;script&gt;")
	assert.Contains(t, result, "&amp; details")
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
		IncludeEvents: true,
		IncludeLogs:   true,
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
		IncludeEvents: true,
		IncludeLogs:   true,
	}

	result := e.FormatText("test-cluster", "Custom text alert")
	assert.Contains(result, "Custom text alert")
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

func TestIsPermanentHTTPStatus(t *testing.T) {
	cases := map[int]bool{
		http.StatusOK:              false,
		http.StatusBadRequest:      true, // the payload is wrong
		http.StatusUnauthorized:    true, // the token is wrong
		http.StatusForbidden:       true,
		http.StatusNotFound:        true,  // the channel/webhook is wrong
		http.StatusRequestTimeout:  false, // explicitly asks to try again
		http.StatusTooManyRequests: false, // rate limited, has Retry-After
		// the server is having a bad time
		http.StatusInternalServerError: false,
		http.StatusBadGateway:          false,
		http.StatusServiceUnavailable:  false,
	}
	for code, want := range cases {
		if got := IsPermanentHTTPStatus(code); got != want {
			t.Errorf("status %d: permanent=%v, want %v", code, got, want)
		}
	}
}

func TestCheckHTTPResponseAcceptsOnlyTwoHundredStatuses(t *testing.T) {
	for _, status := range []int{http.StatusContinue, http.StatusMultipleChoices} {
		resp := &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     http.Header{},
		}
		assert.Error(t, CheckHTTPResponse(resp, "test"))
	}
	resp := &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     http.Header{},
	}
	assert.NoError(t, CheckHTTPResponse(resp, "test"))
}
