package config

import "testing"

// Jitter is on unless a provider turns it off: instances alerting into
// one shared provider must not retry in lockstep after a blip.
func TestCompileRetryJittersByDefault(t *testing.T) {
	if got := compileRetry(map[string]interface{}{}); !got.JitterEnabled {
		t.Errorf("default policy %+v has no jitter", got)
	}
	off := compileRetry(map[string]interface{}{
		"retry": map[string]interface{}{"jitterEnabled": false}})
	if off.JitterEnabled {
		t.Errorf("jitterEnabled=false is not honored: %+v", off)
	}
}
