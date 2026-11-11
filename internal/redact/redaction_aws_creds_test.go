package redact

import "testing"

// The errors of the AWS credential sources (SES, SNS) stay readable:
// they name the file, role and code, never a secret.
func TestRedactKeepsAWSCredentialErrors(t *testing.T) {
	for _, text := range []string{
		"SNS: AWS session token expired (ExpiredToken): kwatch cannot " +
			"refresh a configured sessionToken; use IAM roles for " +
			"service accounts or EKS Pod Identity",
		"SNS: AWS refused the credentials (InvalidClientTokenId); " +
			"fetching new ones and retrying",
		"read token file /var/run/secrets/eks.amazonaws.com/" +
			"serviceaccount/token is empty",
		"assume role arn:aws:iam::123456789012:role/kwatch with web " +
			"identity: (STS code InvalidIdentityToken)",
	} {
		if got := Credentials(text); got != text {
			t.Errorf("changed:\n got %q\nwant %q", got, text)
		}
	}
}

// What the AWS answers and requests carry is redacted if it ever
// reaches a log line.
func TestRedactAWSTemporaryCredentials(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"security token header",
			"X-Amz-Security-Token: FwoGZXIvYXdzEJr0123456789abcdef",
			"X-Amz-Security-Token: [redacted]"},
		{"web identity token form",
			"WebIdentityToken=eyJhbGciOiJSUzI1NiIsImtpZCI6IjEifQ",
			"WebIdentityToken=[redacted]"},
		{"session token json",
			`{"Token": "IQoJb3JpZ2luX2VjEJr0123456789abcdef"}`,
			`{"Token": "[redacted]"}`},
	})
}
