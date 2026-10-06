package pipeline

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/klog/v2"
)

// The restore line says how many incidents came back from the state
// file and how long reading them took.
func TestRestoreLogsTheRestoredIncidentCount(t *testing.T) {
	var out bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&out)
	t.Cleanup(func() {
		klog.Flush()
		klog.SetOutput(nil)
		klog.LogToStderr(true)
	})

	h := restartedHarness(t)
	klog.Flush()

	logged := out.String()
	assert.Contains(t, logged, `"restored state"`)
	assert.Contains(t, logged, "incidents=1")
	assert.Contains(t, logged, "durationMs=")
	assert.NotNil(t, h)
}
