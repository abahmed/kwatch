package kubeclient

import (
	"bytes"
	"strings"
	"testing"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
)

func TestNewHTTPClientInvalidProxyLogsNoCredentials(t *testing.T) {
	var out bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&out)
	t.Cleanup(func() {
		klog.Flush()
		klog.SetOutput(nil)
		klog.LogToStderr(true)
	})

	client := NewHTTPClient(config.ApplicationRuntime{
		ProxyURL: "http://user:s3cret@proxy:bad-port/",
	})
	klog.Flush()

	if client == nil {
		t.Fatal("client is nil")
	}
	logged := out.String()
	if !strings.Contains(logged, "invalid outbound proxy URL") {
		t.Fatalf("missing proxy error log: %q", logged)
	}
	if strings.Contains(logged, "s3cret") || strings.Contains(logged, "user:") {
		t.Fatalf("proxy credentials logged: %q", logged)
	}
}
