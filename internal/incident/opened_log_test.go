package incident

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func captureKlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var out bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&out)
	t.Cleanup(func() {
		klog.Flush()
		klog.SetOutput(nil)
		klog.LogToStderr(true)
	})
	return &out
}

// An incident logs its open once at the default level, with its root,
// reason, tier and send time, however many findings join it.
func TestIncidentOpenLoggedOncePerIncident(t *testing.T) {
	out := captureKlog(t)
	r := newRig(t, Config{})
	pod := podSig("web")
	other := sig(entity(kube.KindPod, "web"),
		reasons.OOMKilled, detection.Warning)

	r.raise(at(0), pod)
	r.raise(at(time.Second), other)
	r.raise(at(2*time.Second), podSig("api"))
	klog.Flush()

	logged := out.String()
	assert.Equal(t, 2, strings.Count(logged, `"incident opened"`))
	web := lineWith(logged, `name="web"`)
	assert.Contains(t, web, `namespace="shop"`)
	assert.Contains(t, web, `kind="pod"`)
	assert.Contains(t, web, `reason="`+reasons.CrashLoopBackOff+`"`)
	assert.Contains(t, web, `incident="`+r.idOf(pod.Entity)+`"`)
	assert.Contains(t, web, `tier=`)
	assert.Contains(t, web,
		`sendsAt="`+at(DefaultSettle).UTC().Format(time.RFC3339)+`"`)
}

func lineWith(logged, needle string) string {
	for _, line := range strings.Split(logged, "\n") {
		if strings.Contains(line, "incident opened") &&
			strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}
