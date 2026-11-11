package delivery

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// captureWarnings returns what fn logs.
func captureWarnings(t *testing.T, fn func()) string {
	t.Helper()
	var out bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&out)
	defer func() {
		klog.Flush()
		klog.SetOutput(nil)
		klog.LogToStderr(true)
	}()
	fn()
	klog.Flush()
	return out.String()
}

// A pager or tracker with no cluster name is reported at start: two
// such clusters on one account could merge or resolve each other's
// alerts.
func TestEmptyClusterNameWithAKeyedProviderIsReported(t *testing.T) {
	pager := providerEntry{provider: &skippingProvider{
		errorRecorderProvider{name: "PagerDuty"}}}
	chat := providerEntry{provider: &errorRecorderProvider{name: "Chat"}}

	logged := captureWarnings(t, func() {
		warnUnnamedCluster([]providerEntry{chat, pager}, "  ")
	})
	assert.Contains(t, logged, "app.clusterName is empty")
	assert.Contains(t, logged, "PagerDuty")
	assert.NotContains(t, logged, "Chat")

	assert.Empty(t, captureWarnings(t, func() {
		warnUnnamedCluster([]providerEntry{chat, pager}, "east")
	}), "a named cluster has nothing to report")
	assert.Empty(t, captureWarnings(t, func() {
		warnUnnamedCluster([]providerEntry{chat}, "")
	}), "chat providers do not track alerts by key")
}

// The report is made when the providers are built.
func TestInitRuntimeReportsAnUnnamedCluster(t *testing.T) {
	manager := newTestManager()
	logged := captureWarnings(t, func() {
		err := initTestManager(manager,
			map[string]map[string]interface{}{"pagerduty": {}}, nil,
			func(string, map[string]interface{},
				transport.ProviderContext) Provider {
				return &skippingProvider{
					errorRecorderProvider{name: "PagerDuty"}}
			})
		assert.NoError(t, err)
	})
	assert.Contains(t, logged, "app.clusterName is empty")
}
