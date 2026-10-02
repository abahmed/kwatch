package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactAdditionalSecretForms(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"password with at and slash",
			"postgres://user:p@ss/word@host/db",
			"postgres://[redacted]@host/db"},
		{"url query keeps email", "https://h.test/x?e=a@b.c",
			"https://h.test/x?e=a@b.c"},
		{"camel secretAccessKey", "secretAccessKey=abcd1234",
			"secretAccessKey=[redacted]"},
		{"camel privateKey json", `{"privateKey":"abc"}`,
			`{"privateKey":"[redacted]"}`},
		{"passphrase", "passphrase: hunter2", "passphrase: [redacted]"},
		{"pwd connection string", "Server=db;Pwd=s3cret;",
			"Server=db;Pwd=[redacted];"},
		{"credentials key", "credentials=abc", "credentials=[redacted]"},
		{"cookie header", "Cookie: sid=abc; theme=dark",
			"Cookie: [redacted]"},
		{"set-cookie header", "Set-Cookie: sid=abc; Path=/; HttpOnly",
			"Set-Cookie: [redacted]"},
		{"flag with space", "run --password hunter2 --verbose",
			"run --password [redacted] --verbose"},
		{"flag followed by flag", "run --password --verbose",
			"run --password --verbose"},
		{"space separated password", "login password hunter2 failed",
			"login password [redacted] failed"},
		{"mysql glued password", "mysql -u root -pSECRET db",
			"mysql -u root -p[redacted] db"},
		{"mysqldump", "mysqldump -h x -pS3cr3t db",
			"mysqldump -h x -p[redacted] db"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Credentials(tc.in))
		})
	}
}

func TestRedactKeepsProseAroundSecretWords(t *testing.T) {
	for _, in := range []string{
		"failed to get secret: not found",
		"invalid token: expired",
		"invalid credentials: user not found",
		"password policy requires 12 characters",
		"password is required",
		"PWD=/home/app",
		"mysql --port 3306 -u root",
		"mysql -p 3306",
		"token: unauthorized",
	} {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, in, Credentials(in))
		})
	}
}

func TestRedactColonSecretWithCredentialShape(t *testing.T) {
	assert.Equal(t, "secret: [redacted]", Credentials("secret: abc12345"))
	assert.Equal(t, "token: [redacted]", Credentials("token: abc123"))
	assert.Equal(t, "secret: [redacted]",
		Credentials(`secret: "two words"`))
}
