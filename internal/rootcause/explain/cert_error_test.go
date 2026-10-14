package explain

import "testing"

func TestCertificateErrorQuotesOnlyTheX509Phrase(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		`Post "https://a": x509: certificate has expired or is not yet ` +
			`valid`: "x509: certificate has expired or is not yet valid",
		"x509: certificate signed by unknown authority": "x509: " +
			"certificate signed by unknown authority",
		"connection refused":      "",
		"x509: unknown flag":      "",
		"certificate will expire": "",
	}
	for text, want := range cases {
		if got := CertificateError(text); got != want {
			t.Errorf("CertificateError(%q) = %q, want %q", text, got, want)
		}
	}
}
