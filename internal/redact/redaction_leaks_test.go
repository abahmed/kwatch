package redact

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Every row is text a pod can print; secret must not survive redaction.
func TestRedactCredentialsRemovesTheReviewedLeaks(t *testing.T) {
	rows := []struct{ name, in, secret string }{
		{"PGPASSWORD", "PGPASSWORD=hunter2 psql", "hunter2"},
		{"DBPASSWORD", "DBPASSWORD=hunter2", "hunter2"},
		{"GITHUBTOKEN", "GITHUBTOKEN=abc123def", "abc123def"},
		{"SMTP_PASS", "SMTP_PASS=hunter2", "hunter2"},
		{"pass=", "pass=abc123", "abc123"},
		{"PASS:", "PASS: abc123", "abc123"},
		{"pass in pairs", "user=admin pass=Zx9wq1", "Zx9wq1"},
		{"passwort", "passwort=abc123", "abc123"},
		{"sslpassword", "sslpassword=abc", "abc"},
		{"curl -u", "curl -u admin:s3cretPw https://api.example.com",
			"s3cretPw"},
		{"curl --user", "curl --user admin:s3cretPw https://x", "s3cretPw"},
		{"curl --user=", "curl --user=admin:s3cretPw https://x", "s3cretPw"},
		{"redis-cli -a", "redis-cli -a hunter2 ping", "hunter2"},
		{"sshpass -p", "sshpass -p hunter2 ssh x", "hunter2"},
		{"docker login -p", "docker login -u x -p hunter2", "hunter2"},
		{"azure account key",
			"AccountKey=abc123def456+/ghi==;EndpointSuffix=core.windows.net",
			"abc123def456"},
		{"azure connection string",
			"DefaultEndpointsProtocol=https;AccountName=x;" +
				"AccountKey=abc123def456ghi789/jkl==;", "abc123def456"},
		{"sas sig", "https://a.blob.core.windows.net/c?sv=2020&sig=abcDEF%2Bghi",
			"abcDEF"},
		{"bare sig", "sig=abc123def456", "abc123def456"},
		{"signature query", "https://x.test/hook?signature=abcdef123456",
			"abcdef123456"},
		{"X-Amz-Signature", "X-Amz-Signature=abcdef0123456789abcdef",
			"abcdef0123456789"},
		{"X-Goog-Signature", "https://x.test/?X-Goog-Signature=abcdef123456",
			"abcdef123456"},
		{"sessionid", "sessionid=abcdef0123", "abcdef0123"},
		{"SESSION", "SESSION=abcdef0123456", "abcdef0123456"},
		{"docker auth json", `{"auths":{"r.io":{"auth":"dXNlcjpwYXNz"}}}`,
			"dXNlcjpwYXNz"},
		{"docker auth key", `"auth": "dXNlcjpwYXNzd29yZA=="`,
			"dXNlcjpwYXNzd29yZA"},
		{"docker auth equals",
			"unauthorized: incorrect username or password; " +
				"auth=dXNlcjpwYXNzd29yZA==", "dXNlcjpwYXNzd29yZA"},
		{"dockerconfigjson",
			"dockerconfigjson: eyJhdXRocyI6eyJodHRwczovL2luZGV4LmRvY2tlci5pby" +
				"92MS8iOnsidXNlcm5hbWUiOiJ4IiwicGFzc3dvcmQiOiJ5In19fQ==",
			"eyJhdXRocyI6"},
		{"pgp private key",
			"-----BEGIN PGP PRIVATE KEY BLOCK-----\nlQdGBF\n" +
				"-----END PGP PRIVATE KEY BLOCK-----", "lQdGBF"},
		{"stripe restricted", "rk_" + "live_abcdefghijklmnopqrstuvwx",
			"abcdefghijklmnop"},
		{"grafana", "gl" + "sa_abcdefghijklmnopqrstuvwxyz0123456789_abcdef01",
			"abcdefghijklmnop"},
		{"vault", "hv" + "s.CAESIabcdefghijklmnopqrstuv", "CAESIabcdef"},
		{"digitalocean",
			"dop_" +
				"v1_0123456789abcdef0123456789abcdef0123456789abcdef01234567",
			"0123456789abcdef"},
		{"age", "AGE-SECRET-" +
			"KEY-1ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789ABCDEF",
			"ABCDEFGHIJKLMNOP"},
		{"token userinfo", "https://abcd1234token@example.com/path",
			"abcd1234token"},
		{"user-only userinfo", "https://token@example.com/path", "token@"},
		{"password with separators", "password=a,b;c", "b;c"},
		{"digest response",
			`Authorization: Digest username="x", response="abcdef0123"`,
			"abcdef0123"},
		{"password is", "the admin password is swordfish99 today",
			"swordfish99"},
		{"bearer colon", "bearer: abc123xyz", "abc123xyz"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			got := Credentials(row.in)
			assert.NotContains(t, got, row.secret, "from %q", row.in)
		})
	}
}

func TestRedactCredentialsKeepsOrdinaryText(t *testing.T) {
	for _, in := range []string{
		"docker run --user 1000:1000 busybox",
		"curl -u root http://example.com",
		"ssh://git@github.com/org/repo.git",
		"git@github.com:org/repo.git",
		"3 tests pass: 4 fail: 1",
		"bypass=true compass=north",
		"password is required",
		"the password is too short",
		"signature: invalid",
		"sig=SIGTERM",
		"session=ended",
		"redis-cli ping",
		"ls -a /tmp",
		"sshpass is not installed",
	} {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, in, Credentials(in))
		})
	}
}

func TestRedactKeepsTheRestOfAConnectionString(t *testing.T) {
	assert.Equal(t, "Server=db;Password=[redacted];User=x",
		Credentials("Server=db;Password=Hunt3r2;User=x"))
	assert.Equal(t, "password=[redacted], retrying",
		Credentials("password=abc123, retrying"))
}

func TestRedactMasksTheAddressRangesPodsAndCloudsUse(t *testing.T) {
	for _, ip := range []string{"100.64.0.1", "100.127.255.254",
		"169.254.169.254", "10.1.2.3", "172.16.0.9", "192.168.1.2"} {
		got := Evidence("dial tcp " + ip + ":5432: refused")
		assert.True(t, strings.Contains(got, "[private-address]"),
			"%s: %s", ip, got)
		assert.Equal(t, "dial tcp "+ip+":5432: refused",
			Credentials("dial tcp "+ip+":5432: refused"),
			"credentials only keeps addresses")
	}
	for _, ip := range []string{"100.63.0.1", "100.128.0.1", "169.253.1.1",
		"8.8.8.8"} {
		assert.Equal(t, "dial tcp "+ip+":53", Evidence("dial tcp "+ip+":53"))
	}
}
