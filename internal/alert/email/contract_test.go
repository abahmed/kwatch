package email

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"

	gomail "gopkg.in/mail.v2"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// mailHeaders are the message headers the golden pins.
var mailHeaders = []string{"From", "To", "Subject", "Mime-Version",
	"Content-Transfer-Encoding", "X-Kwatch-Cluster",
	"X-Kwatch-Alert-Key"}

// TestContract pins the RFC 5322 message the SMTP provider builds. SMTP
// has no HTTP request, so each built message is relayed to the recorder
// as an "SMTP" request: headers decoded, body quoted-printable decoded.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewEmail(map[string]interface{}{
		"from": "kwatch@example.com",
		"to":   "ops@example.com,oncall@example.com",
		"host": "127.0.0.1", "port": "587",
	}, "dev", transport.Dependencies{})
	c.send = func(messages ...*gomail.Message) error {
		for _, m := range messages {
			if err := relay(rec, m); err != nil {
				return err
			}
		}
		return nil
	}
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers: mailHeaders, LimitBytes: 10000,
		KeyOf: func(t *testing.T, r providertest.Request) string {
			return r.Header.Get("X-Kwatch-Alert-Key")
		},
		Check: checkMessage,
	})
}

// relay renders m as a mail reader sees it and posts it to the recorder.
func relay(rec *providertest.Recorder, m *gomail.Message) error {
	var raw bytes.Buffer
	if _, err := m.WriteTo(&raw); err != nil {
		return err
	}
	parsed, err := mail.ReadMessage(&raw)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(quotedprintable.NewReader(parsed.Body))
	if err != nil {
		return err
	}
	headers := map[string]string{}
	dec := new(mime.WordDecoder)
	for name, values := range parsed.Header {
		if name == "Date" || name == "Content-Type" || len(values) == 0 {
			continue
		}
		v := values[0]
		if d, err := dec.DecodeHeader(v); err == nil {
			v = d
		}
		headers[name] = v
	}
	_, err = transport.NewSender(rec.Dependencies()).Send(
		context.Background(), transport.Request{
			Provider: "SMTP", Method: "SMTP", URL: rec.URL(), Body: body,
			ContentType: parsed.Header.Get("Content-Type"),
			Headers:     headers,
		})
	return err
}

func checkMessage(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "SMTP" {
		t.Fatalf("messages = %+v", reqs)
	}
	h := reqs[0].Header
	if h.Get("From") != "kwatch@example.com" ||
		h.Get("To") != "ops@example.com, oncall@example.com" {
		t.Fatalf("From or To wrong: %v", h)
	}
	subject := h.Get("Subject")
	if strings.ContainsAny(subject, "\r\n") {
		t.Fatalf("subject must be one line: %q", subject)
	}
	providertest.AssertOneLeadingEmoji(t, subject)
	if !strings.HasPrefix(h.Get("Content-Type"), "text/plain") {
		t.Fatalf("Content-Type = %q", h.Get("Content-Type"))
	}
	if strings.TrimSpace(string(reqs[0].Body)) == "" {
		t.Fatal("body is empty")
	}
	if h.Get("X-Kwatch-Cluster") != "dev" {
		t.Fatalf("cluster header = %q", h.Get("X-Kwatch-Cluster"))
	}
	if h.Get("X-Kwatch-Alert-Key") == "" {
		t.Fatal("alert key header is missing")
	}
}
