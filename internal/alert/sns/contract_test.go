package sns

import (
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/signing"
	"github.com/abahmed/kwatch/internal/notification"
)

const (
	testKeyID  = "AKIA123"
	testSecret = "sns-secret-key"
)

var authShape = regexp.MustCompile(
	`^AWS4-HMAC-SHA256 Credential=AKIA123/20260102/us-east-1/sns/` +
		`aws4_request, SignedHeaders=content-type;host;x-amz-date, ` +
		`Signature=[0-9a-f]{64}$`)

// TestContract pins the exact SNS Publish query request.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewSns(map[string]interface{}{
		"accessKeyId": testKeyID, "secretAccessKey": testSecret,
		"topicArn": topic,
	}, "dev", rec.Dependencies())
	c.url = rec.URL() + "/"
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Secrets: []string{testSecret},
		KeyOf: func(t *testing.T, r providertest.Request) string {
			form, _ := url.ParseQuery(string(r.Body))
			return form.Get("MessageAttributes.entry.1.Value.StringValue")
		},
		Check: func(t *testing.T, name string,
			reqs []providertest.Request) {
			checkPublish(t, rec.URL()+"/", name, reqs)
		},
	})
}

func checkPublish(
	t *testing.T, endpoint, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	r := reqs[0]
	auth := r.Header.Get("Authorization")
	if !authShape.MatchString(auth) ||
		r.Header.Get("X-Amz-Date") != "20260102T030405Z" {
		t.Fatalf("auth = %q, date = %q", auth,
			r.Header.Get("X-Amz-Date"))
	}
	want, err := signing.SignAWSV4At(signing.Credentials{
		AccessKeyID: testKeyID, SecretAccessKey: testSecret,
	}, "us-east-1", "sns", "POST", endpoint, r.Body, providertest.Now)
	if err != nil || want["Authorization"] != auth {
		t.Fatalf("signature does not verify: %v", err)
	}
	form, err := url.ParseQuery(string(r.Body))
	if err != nil {
		t.Fatal(err)
	}
	if form.Get("Action") != "Publish" ||
		form.Get("Version") != "2010-03-31" ||
		form.Get("TopicArn") != topic || form.Get("TargetArn") != "" {
		t.Fatalf("Publish fields wrong: %v", form)
	}
	checkMarker(t, form.Get("Message"))
	checkSubject(t, form.Get("Subject"))
	for i, attr := range []string{"kwatch.key", "kwatch.status"} {
		prefix := "MessageAttributes.entry." + string(rune('1'+i))
		if form.Get(prefix+".Name") != attr ||
			form.Get(prefix+".Value.DataType") != "String" ||
			form.Get(prefix+".Value.StringValue") == "" {
			t.Fatalf("attribute %s wrong: %v", attr, form)
		}
	}
	wantStatus := map[string]string{"resolve": "resolved"}[name]
	got := form.Get("MessageAttributes.entry.2.Value.StringValue")
	if wantStatus != "" && got != wantStatus {
		t.Fatalf("status attribute = %q, want %q", got, wantStatus)
	}
}

// checkSubject enforces what SNS accepts: printable ASCII, one line,
// under 100 characters.
func checkSubject(t *testing.T, subject string) {
	if subject == "" || len(subject) > maxSubjectBytes {
		t.Fatalf("subject length %d: %q", len(subject), subject)
	}
	if strings.Contains(subject, "  ") || subject[0] == ' ' ||
		subject[len(subject)-1] == ' ' {
		t.Fatalf("subject has gaps: %q", subject)
	}
	if strings.ContainsAny(subject, "\r\n") {
		t.Fatalf("subject has a line break: %q", subject)
	}
	for _, r := range subject {
		if r < 0x20 || r > 0x7e {
			t.Fatalf("subject is not printable ASCII: %q", subject)
		}
	}
}

// checkMarker requires the message to start with a status marker; the
// unicode case carries a second emoji in its own text on purpose.
func checkMarker(t *testing.T, message string) {
	for _, marker := range notification.Markers() {
		if strings.HasPrefix(message, marker) {
			return
		}
	}
	t.Fatalf("message has no leading status marker: %.60q", message)
}
