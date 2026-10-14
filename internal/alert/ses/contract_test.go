package ses

import (
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/signing"
)

const (
	testKeyID  = "AKIA123"
	testSecret = "ses-secret-key"
)

var authShape = regexp.MustCompile(
	`^AWS4-HMAC-SHA256 Credential=AKIA123/20260102/us-east-1/ses/` +
		`aws4_request, SignedHeaders=content-type;host;x-amz-date, ` +
		`Signature=[0-9a-f]{64}$`)

// TestContract pins the exact SES SendEmail query request.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewSes(map[string]interface{}{
		"accessKeyId": testKeyID, "secretAccessKey": testSecret,
		"from": "kwatch@example.com",
		"to":   "ops@example.com, oncall@example.com",
	}, "dev", rec.Dependencies())
	c.url = rec.URL()
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Secrets:    []string{testSecret},
		LimitBytes: 10000,
		Check: func(t *testing.T, name string,
			reqs []providertest.Request) {
			checkSendEmail(t, rec.URL(), reqs)
		},
	})
}

func checkSendEmail(
	t *testing.T, endpoint string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	r := reqs[0]
	auth := r.Header.Get("Authorization")
	if !authShape.MatchString(auth) {
		t.Fatalf("Authorization shape = %q", auth)
	}
	if got := r.Header.Get("X-Amz-Date"); got != "20260102T030405Z" {
		t.Fatalf("X-Amz-Date = %q, want the fixed clock", got)
	}
	want, err := signing.SignAWSV4At(signing.Credentials{
		AccessKeyID: testKeyID, SecretAccessKey: testSecret,
	}, "us-east-1", "ses", "POST", endpoint, r.Body,
		providertest.Now)
	if err != nil || want["Authorization"] != auth {
		t.Fatalf("signature does not verify: %v", err)
	}
	form, err := url.ParseQuery(string(r.Body))
	if err != nil {
		t.Fatal(err)
	}
	if form.Get("Action") != "SendEmail" ||
		form.Get("Version") != "2010-12-01" ||
		form.Get("Source") != "kwatch@example.com" ||
		form.Get("Destination.ToAddresses.member.1") !=
			"ops@example.com" ||
		form.Get("Destination.ToAddresses.member.2") !=
			"oncall@example.com" {
		t.Fatalf("SendEmail fields wrong: %v", form)
	}
	subject := form.Get("Message.Subject.Data")
	if subject == "" || strings.ContainsAny(subject, "\r\n") {
		t.Fatalf("subject must be one line: %q", subject)
	}
	providertest.AssertOneLeadingEmoji(t, subject)
	if strings.TrimSpace(form.Get("Message.Body.Text.Data")) == "" ||
		form.Get("Message.Subject.Charset") != "UTF-8" ||
		form.Get("Message.Body.Text.Charset") != "UTF-8" {
		t.Fatalf("body or charset missing: %v", form)
	}
}
