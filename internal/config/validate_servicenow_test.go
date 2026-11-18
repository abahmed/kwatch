package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serviceNowConfig(settings map[string]interface{}) *Config {
	cfg := DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{"servicenow": settings}
	return cfg
}

func TestValidateServiceNowAcceptsOneCredentialPair(t *testing.T) {
	for name, settings := range map[string]map[string]interface{}{
		"basic": {"instanceURL": "https://acme.service-now.com",
			"user": "kwatch", "password": "${file:/s/pw}"},
		"oauth": {"instanceURL": "https://acme.service-now.com/",
			"clientID": "id", "clientSecret": "${file:/s/secret}"},
		"custom domain": {"instanceURL": "https://itsm.example.com:8443",
			"user": "kwatch", "password": "${file:/s/pw}"},
	} {
		assert.Empty(t, validateServiceNow(serviceNowConfig(settings)),
			name)
	}
}

func TestValidateServiceNowRejectsBadSettings(t *testing.T) {
	cases := []struct {
		name     string
		settings map[string]interface{}
		want     string
	}{
		{"plain http", map[string]interface{}{
			"instanceURL": "http://acme.service-now.com",
			"user":        "u", "password": "p"}, "https address"},
		{"path", map[string]interface{}{
			"instanceURL": "https://acme.service-now.com/api/now",
			"user":        "u", "password": "p"}, "without a path"},
		{"user info", map[string]interface{}{
			"instanceURL": "https://u:p@acme.service-now.com",
			"user":        "u", "password": "p"}, "user information"},
		{"no credentials", map[string]interface{}{
			"instanceURL": "https://acme.service-now.com"},
			"set user and password"},
		{"half basic", map[string]interface{}{
			"instanceURL": "https://acme.service-now.com",
			"user":        "u"}, "both user and password"},
		{"half oauth", map[string]interface{}{
			"instanceURL":  "https://acme.service-now.com",
			"clientSecret": "s"}, "both clientID and clientSecret"},
		{"both kinds", map[string]interface{}{
			"instanceURL": "https://acme.service-now.com",
			"user":        "u", "password": "p",
			"clientID": "i", "clientSecret": "s"}, "not both"},
	}
	for _, tc := range cases {
		errs := errorTexts(validateServiceNow(serviceNowConfig(tc.settings)))
		require.Len(t, errs, 1, tc.name)
		assert.Contains(t, errs[0], tc.want, tc.name)
		assert.Contains(t, errs[0], "alert.servicenow", tc.name)
	}
}

func TestValidateServiceNowRequiresInstanceURL(t *testing.T) {
	errs := errorTexts(Validate(serviceNowConfig(map[string]interface{}{
		"user": "u", "password": "p"})))
	assert.Contains(t, errs,
		"alert.servicenow.instanceURL is required and must not be empty")
}

func TestServiceNowCredentialsMustBeFileReferences(t *testing.T) {
	for _, field := range []string{"password", "clientSecret"} {
		raw := "alert:\n  servicenow:\n    " + field + ": plain-" +
			"value\n"
		err := validateSecretReferences(raw)
		require.Error(t, err, field)
		assert.Contains(t, err.Error(), field)
		assert.NotContains(t, err.Error(), "plain-value")
	}
	ok := "alert:\n  servicenow:\n    password: ${file:/s/pw}\n" +
		"    clientSecret: ${file:/s/secret}\n"
	assert.NoError(t, validateSecretReferences(ok))
}
