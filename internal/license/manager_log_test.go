package license

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"k8s.io/klog/v2"
)

func captureKlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var out bytes.Buffer
	klog.LogToStderr(false)
	// Only the INFO stream: it carries every severity once, where
	// SetOutput would print a warning twice.
	klog.SetOutputBySeverity("INFO", &out)
	t.Cleanup(func() {
		klog.Flush()
		klog.SetOutput(nil)
		klog.LogToStderr(true)
	})
	return &out
}

// A sync logs only when its outcome changes: the first success, a
// failure once per reason, and the recovery. The key is never logged.
func TestManagerLogsSyncOutcomeChangesOnly(t *testing.T) {
	out := captureKlog(t)
	h := newHarness(t)
	ctx := context.Background()
	h.m.init(ctx)
	cycles := func(n int) string {
		for range n {
			h.m.cycle(ctx)
		}
		klog.Flush()
		got := out.String()
		out.Reset()
		return got
	}

	first := cycles(2)
	if strings.Count(first, "license confirmed") != 1 {
		t.Fatalf("want one confirmation line, got:\n%s", first)
	}
	for _, want := range []string{`lid="lic_…`, `plan="Pro"`,
		"licenseExpires=", "leaseExpires="} {
		if !strings.Contains(first, want) {
			t.Fatalf("confirmation lacks %s:\n%s", want, first)
		}
	}

	h.worker.set(func(string) (int, map[string]any, map[string]string) {
		return 503, map[string]any{"error": "unavailable " + h.key}, nil
	})
	failed := cycles(2)
	if strings.Count(failed, "license check with the license "+
		"server failed") != 1 || !strings.HasPrefix(failed, "W") {
		t.Fatalf("want one warning, got:\n%s", failed)
	}

	h.worker.set(nil)
	if got := cycles(1); strings.Count(got, "license confirmed") != 1 {
		t.Fatalf("recovery not logged once:\n%s", got)
	}
	if strings.Contains(first+failed, h.key) {
		t.Fatal("license key logged")
	}
}
