package message

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactJsonPassword(t *testing.T) {
	input := `{"password": "hunter2", "user": "admin"}`
	got := RedactEvidence(input)
	assert.NotContains(t, got, "hunter2")
	assert.Contains(t, got, "\"password\"")
	assert.Contains(t, got, "admin")
}

func TestRedactJsonClientSecret(t *testing.T) {
	input := `{"client_secret":"secret123","id":"abc"}`
	got := RedactEvidence(input)
	assert.NotContains(t, got, "secret123")
	assert.Contains(t, got, "client_secret")
}

func TestRedactURLUserinfo(t *testing.T) {
	input := "postgres://user:pa55@db:5432/mydb"
	got := RedactEvidence(input)
	assert.NotContains(t, got, "user:pa55")
	assert.NotContains(t, got, "pa55")
	assert.Contains(t, got, "postgres://")
	assert.Contains(t, got, "@db:5432")
	assert.Contains(t, got, "[redacted]")
}

func TestRedactAWSKey(t *testing.T) {
	// Built from parts so secret scanners do not flag the fixture.
	key := "AKIA" + "ABCDEFGHIJKLMNOP"
	input := "AWS key " + key + " was found"
	got := RedactEvidence(input)
	assert.NotContains(t, got, key)
	assert.Contains(t, got, "[redacted]")
}

func TestRedactAWSAsiaKey(t *testing.T) {
	input := "AWS temporary key ASIAABCDEFGHIJKLMNOP"
	got := RedactEvidence(input)
	assert.NotContains(t, got, "ASIAABCDEFGHIJKLMNOP")
	assert.Contains(t, got, "[redacted]")
}

func TestRedactJWT(t *testing.T) {
	input := "bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123xyz"
	got := RedactEvidence(input)
	assert.NotContains(t, got, "eyJhbGciOiJIUzI1NiJ9")
	assert.NotContains(t, got, "abc123xyz")
	assert.Contains(t, got, "[redacted]")
}

func TestRedactPEMPrivateKey(t *testing.T) {
	input := "key:\n-----BEGIN RSA PRIVATE KEY-----\nMIIabc==\n" +
		"-----END RSA PRIVATE KEY-----"
	got := RedactEvidence(input)
	assert.NotContains(t, got, "MIIabc==")
	assert.NotContains(t, got, "BEGIN RSA PRIVATE KEY")
	assert.Contains(t, got, "[redacted private key]")
}

func TestRedactPEMECPrivateKey(t *testing.T) {
	input := "-----BEGIN EC PRIVATE KEY-----\nMIGfMA0=\n" +
		"-----END EC PRIVATE KEY-----"
	got := RedactEvidence(input)
	assert.NotContains(t, got, "MIGfMA0=")
	assert.Contains(t, got, "[redacted private key]")
}

func TestRedactIPv6ULAPrivateAddress(t *testing.T) {
	input := "replica on fd12:3456::1 is down"
	got := RedactEvidenceWithPolicy(input, false)
	assert.NotContains(t, got, "fd12:3456::1")
	assert.Contains(t, got, "[private-address]")
}

func TestRedactIPv6LinkLocal(t *testing.T) {
	input := "link-local address fe80::1 discovered"
	got := RedactEvidenceWithPolicy(input, false)
	assert.NotContains(t, got, "fe80::1")
	assert.Contains(t, got, "[private-address]")
}

func TestKeepIPv6PrivateAddressWhenPolicyAllows(t *testing.T) {
	input := "replica on fd12:3456::1 is down"
	got := RedactEvidenceWithPolicy(input, true)
	assert.Contains(t, got, "fd12:3456::1")
}

func TestRedactBearerToken(t *testing.T) {
	input := "Authorization: Bearer abc123def456"
	got := RedactEvidence(input)
	assert.NotContains(t, got, "abc123def456")
	assert.Contains(t, got, "Bearer [redacted]")
}

func TestRedactBasicAuth(t *testing.T) {
	input := "Authorization: Basic dXNlcjpwYXNz"
	got := RedactEvidence(input)
	assert.NotContains(t, got, "dXNlcjpwYXNz")
	assert.Contains(t, got, "Basic [redacted]")
}

func TestRedactQueryParamToken(t *testing.T) {
	input := "https://example.com/hook?token=secret123&x=1"
	got := RedactEvidence(input)
	assert.NotContains(t, got, "secret123")
	assert.Contains(t, got, "[redacted]")
	assert.NotContains(t, got, "x=1")
}

func TestRedactQueryParamPasswordSingleParam(t *testing.T) {
	input := "URL?password=hunter2"
	got := RedactEvidence(input)
	assert.NotContains(t, got, "hunter2")
	assert.Contains(t, got, "[redacted]")
}

func TestRedactMultipleSecrets(t *testing.T) {
	input := "token=abc123 password=hunter2 Bearer secret456 " +
		`{"api_key":"key789"}`
	got := RedactEvidence(input)
	assert.NotContains(t, got, "abc123")
	assert.NotContains(t, got, "hunter2")
	assert.NotContains(t, got, "secret456")
	assert.NotContains(t, got, "key789")
}

func TestRedactColonSeparatedSecret(t *testing.T) {
	input := "password: my-secret-here"
	got := RedactEvidence(input)
	assert.NotContains(t, got, "my-secret-here")
	assert.Contains(t, got, "password")
}

func TestRedactEqualsSeparatedSecret(t *testing.T) {
	input := "api_key = very_sensitive_key"
	got := RedactEvidence(input)
	assert.NotContains(t, got, "very_sensitive_key")
	assert.Contains(t, got, "api_key")
}

func TestNormalPrivateAddressRemovedByDefault(t *testing.T) {
	input := "pod on 10.0.0.5 error: 192.168.1.10 failed"
	got := RedactEvidenceWithPolicy(input, false)
	assert.NotContains(t, got, "10.0.0.5")
	assert.NotContains(t, got, "192.168.1.10")
	assert.Contains(t, got, "[private-address]")
}

func TestNormalPrivateAddressKeptWhenPolicy(t *testing.T) {
	input := "pod on 10.0.0.5"
	got := RedactEvidenceWithPolicy(input, true)
	assert.Contains(t, got, "10.0.0.5")
}

func TestRedactAccessToken(t *testing.T) {
	input := "access-token: abc123"
	got := RedactEvidence(input)
	assert.NotContains(t, got, "abc123")
	assert.Contains(t, got, "access-token")
}

func TestRedactAccessTokenWithUnderscore(t *testing.T) {
	input := "access_token=xyz789"
	got := RedactEvidence(input)
	assert.NotContains(t, got, "xyz789")
}

func TestRedactApiKey(t *testing.T) {
	input := "api-key: super-secret-api-key-123"
	got := RedactEvidence(input)
	assert.NotContains(t, got, "super-secret-api-key-123")
}

func TestRedactHttpsURLUserinfo(t *testing.T) {
	input := "https://admin:secret@api.example.com/v1/query"
	got := RedactEvidence(input)
	assert.NotContains(t, got, "admin:secret")
	assert.Contains(t, got, "https://")
	assert.Contains(t, got, "@api.example.com")
}

func TestRedactionEmptyString(t *testing.T) {
	input := ""
	got := RedactEvidence(input)
	assert.Equal(t, "", got)
}

func TestRedactionNoSecrets(t *testing.T) {
	input := "this is a normal log message with no secrets"
	got := RedactEvidence(input)
	assert.Equal(t, input, got)
}
