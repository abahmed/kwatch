package webhook

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/klog/v2"
)

func hookConfig(extra map[string]interface{}) map[string]interface{} {
	cfg := map[string]interface{}{"url": "https://example.test/hook"}
	for k, v := range extra {
		cfg[k] = v
	}
	return cfg
}

// Bad auth settings must stop startup: a provider that starts without its
// credentials gets a 401 on every alert and dead-letters all of them.
func TestBadAuthSettingsFailConstruction(t *testing.T) {
	cases := map[string]map[string]interface{}{
		"headers not a list": {"headers": "X-Token: abc"},
		"header not a map":   {"headers": []interface{}{"X-Token"}},
		"header without name": {"headers": []interface{}{
			map[string]interface{}{"value": "abc"}}},
		"header name with space": {"headers": []interface{}{
			map[string]interface{}{"name": "X Token", "value": "a"}}},
		"header value with newline": {"headers": []interface{}{
			map[string]interface{}{"name": "X-Token", "value": "a\nb"}}},
		"basicAuth not a map": {"basicAuth": "user:pass"},
		"basicAuth no password": {"basicAuth": map[string]interface{}{
			"username": "user"}},
		"basicAuth no username": {"basicAuth": map[string]interface{}{
			"password": "pass"}},
		"basicAuth wrong type": {"basicAuth": map[string]interface{}{
			"username": 5, "password": "pass"}},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Nil(t, NewWebhook(hookConfig(extra), "dev", testDeps))
		})
	}
}

func TestGoodAuthSettingsConstruct(t *testing.T) {
	good := hookConfig(map[string]interface{}{
		"headers": []interface{}{
			map[string]interface{}{"name": "X-Token", "value": "abc"}},
		"basicAuth": map[string]interface{}{
			"username": "user", "password": "pass"},
	})
	assert.NotNil(t, NewWebhook(good, "dev", testDeps))
	assert.NotNil(t, NewWebhook(hookConfig(nil), "dev", testDeps))
}

func TestCredentialsOverHTTPWarn(t *testing.T) {
	var out bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&out)
	t.Cleanup(func() {
		klog.Flush()
		klog.SetOutput(nil)
		klog.LogToStderr(true)
	})
	cfg := map[string]interface{}{
		"url": "http://example.test/hook",
		"basicAuth": map[string]interface{}{
			"username": "user", "password": "pass"},
	}

	hook := NewWebhook(cfg, "dev", testDeps)
	klog.Flush()

	assert.NotNil(t, hook, "http stays allowed; it only warns")
	assert.Contains(t, out.String(), "credentials over plain http")
	assert.NotContains(t, out.String(), "pass")
}
