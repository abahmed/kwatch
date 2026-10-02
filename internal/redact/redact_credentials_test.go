package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactCredentialsKeepsPrivateAddresses(t *testing.T) {
	tt := []struct {
		name, in, want string
	}{
		{"bearer", "auth Bearer abc.def", "auth Bearer [redacted]"},
		{"basic_url", "http://u:p@10.0.0.1/x", "http://[redacted]@10.0.0.1/x"},
		{"aws_key", "key AKIAIOSFODNN7EXAMPLE", "key [redacted]"},
		{"jwt", "t eyJhbGciOiJIUzI1.eyJzdWIiOiIxMjM0.sig", "t [redacted]"},
		{"query_password", "GET /?password=abc", "GET /?password=[redacted]"},
		{"private_key",
			"-----BEGIN RSA PRIVATE KEY-----\nMIIB\n-----END RSA PRIVATE KEY-----",
			"[redacted private key]"},
		{"plain", "dial 10.1.2.3:80 refused", "dial 10.1.2.3:80 refused"},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Credentials(tc.in))
		})
	}
}
