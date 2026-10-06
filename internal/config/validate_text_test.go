package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func errorTexts(errs []error) []string {
	texts := make([]string, 0, len(errs))
	for _, err := range errs {
		texts = append(texts, err.Error())
	}
	return texts
}

func TestValidateRejectsBrokenGlobalTemplate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Templates = map[string]string{"oomkilled": "{{ .Reason "}

	errs := errorTexts(validateTemplates(cfg))

	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], `templates["oomkilled"]`)
}

func TestValidateRejectsBrokenProviderTemplate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{
		"slack": {"templates": map[string]interface{}{"x": "{{ if }}"}},
	}

	errs := errorTexts(validateTemplates(cfg))

	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "alert.slack.templates")
}

func TestValidateAcceptsGoodTemplates(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Templates = map[string]string{"x": "{{ .Namespace }}"}
	assert.Empty(t, validateTemplates(cfg))
}

func TestValidateRunbookURLs(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Runbooks = map[string]string{
		"good": "https://runbooks.example.com/oom",
		"bad":  "runbooks/oom",
		"ftp":  "ftp://example.com/x",
	}

	errs := errorTexts(validateRunbooks(cfg))

	require.Len(t, errs, 2)
	assert.Contains(t, errs[0], `runbooks["bad"]`)
	assert.Contains(t, errs[1], `runbooks["ftp"]`)
}

func TestValidateFallbackMustBeConfigured(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{
		"slack":   {"fallback": "Discord"},
		"discord": {"fallback": "teams"},
	}

	errs := errorTexts(validateFallbacks(cfg))

	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "alert.discord.fallback")
}

func TestValidateDatadogSite(t *testing.T) {
	good := []string{"", "datadoghq.com", "datadoghq.eu",
		"us3.datadoghq.com", "us5.datadoghq.com", "ddog-gov.com",
		"datadoghq.com:443", "localhost", "my_proxy.internal",
		" datadoghq.eu "}
	bad := []string{"https://datadoghq.com", "datadoghq.com/x",
		"data dog.com", "a?b.com", "a.com#x", "evil.com@x.com"}
	for _, site := range good {
		cfg := DefaultConfig()
		cfg.Alert = map[string]map[string]interface{}{
			"datadog": {"apiKey": "k", "site": site}}
		assert.Empty(t, validateDatadogSite(cfg), "site %q", site)
	}
	for _, site := range bad {
		cfg := DefaultConfig()
		cfg.Alert = map[string]map[string]interface{}{
			"datadog": {"apiKey": "k", "site": site}}
		errs := errorTexts(validateDatadogSite(cfg))
		require.Len(t, errs, 1, "site %q", site)
		assert.Contains(t, errs[0], "alert.datadog.site")
	}
}
