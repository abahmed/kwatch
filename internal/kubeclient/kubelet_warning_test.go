package kubeclient

import (
	"bytes"
	"strings"
	"testing"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
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

func TestInsecureKubeletTransportWarnsAboutTokenExposure(t *testing.T) {
	out := captureKlog(t)
	base := &rest.Config{Host: "https://api.example:6443"}

	if _, err := NewKubeletTransport(base, true); err != nil {
		t.Fatal(err)
	}
	klog.Flush()
	logged := out.String()
	if !strings.Contains(logged, "insecureSkipVerify") ||
		!strings.Contains(logged, "capture that token") {
		t.Fatalf("missing warning: %q", logged)
	}
}

func TestVerifiedKubeletTransportDoesNotWarn(t *testing.T) {
	out := captureKlog(t)
	base := &rest.Config{Host: "https://api.example:6443"}

	if _, err := NewKubeletTransport(base, false); err != nil {
		t.Fatal(err)
	}
	klog.Flush()
	if strings.Contains(out.String(), "insecureSkipVerify") {
		t.Fatalf("unexpected warning: %q", out.String())
	}
}
