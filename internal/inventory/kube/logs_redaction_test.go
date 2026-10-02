package kube

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const pemBody = "MIIEowIBAAKCAQEAu1SU1LfVLPHCozMxH2Mo4lgOEePzNm0tRgeL" +
	"ezV6ffAt0gun\n" +
	"VTLw7onLRnrq0/IzW7yWR7QkrmBL7jTKEn5u+qKhbwKfBstIs+bMY2Zkp18gnTxK\n"

func TestLogLinesRedactsMultiLinePrivateKey(t *testing.T) {
	body := "starting\nerror: bad config\n" +
		"-----BEGIN RSA PRIVATE KEY-----\n" + pemBody +
		"-----END RSA PRIVATE KEY-----\n"

	lines := logLines([]byte(body))

	joined := strings.Join(lines, "\n")
	assert.NotContains(t, joined, "MIIEowIBAAKCAQEA")
	assert.NotContains(t, joined, "VTLw7onLRnrq0")
	assert.Contains(t, joined, "[redacted private key]")
	assert.Equal(t, "error: bad config", lines[1])
}

func TestLogLinesRedactsUnterminatedKeyAtEnd(t *testing.T) {
	body := "boot\n-----BEGIN PRIVATE KEY-----\n" + pemBody

	joined := strings.Join(logLines([]byte(body)), "\n")

	assert.NotContains(t, joined, "MIIEowIBAAKCAQEA")
	assert.Contains(t, joined, "boot")
}

// The log tail can start inside a key, after its BEGIN line.
func TestLogLinesDropsBareBase64Runs(t *testing.T) {
	body := pemBody + "-----END RSA PRIVATE KEY-----\n" +
		"panic: short token abc123\n"

	lines := logLines([]byte(body))

	assert.NotContains(t, strings.Join(lines, "\n"), "MIIEowIBAAKCAQEA")
	assert.Contains(t, lines, "panic: short token abc123")
}
