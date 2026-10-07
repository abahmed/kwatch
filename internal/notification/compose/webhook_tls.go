package compose

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// tlsCause reports a cause that is an admission webhook whose calls fail
// the TLS handshake.
func tlsCause(cause *rootcause.CauseRecord) bool {
	return cause != nil && cause.Mode == explain.ModeWebhookTLS &&
		webhookKind(cause.Root.Kind)
}

// tlsError matches the TLS error in a failed webhook call, from where
// it starts to the end of the message.
var tlsError = regexp.MustCompile(
	`(x509: .*|tls: (?:bad|expired|unknown) certificate.*)`)

// tlsLead blames the webhook and quotes what the API server said:
// "gateway in payments cannot create pods because validating webhook
// image-policy rejects calls: "x509: certificate signed by unknown
// authority"". It is false for any other cause.
func tlsLead(f caseFacts) (string, bool) {
	if !tlsCause(f.p.Cause) {
		return "", false
	}
	subject := symptomSubject(f)
	why := nameFrom(subject, f.p.Cause.Root) + " rejects calls"
	if reason := tlsReason(f); reason != "" {
		why += ": " + quoted(reason)
	}
	return causeLink(f.p.Cause, symptomState(f, subject), why), true
}

// tlsReason is the TLS error in the first member message that has one,
// as the API server worded it.
func tlsReason(f caseFacts) string {
	for _, m := range f.members {
		texts := []string{m.Summary}
		for _, e := range m.Evidence {
			texts = append(texts, e.Value)
		}
		for _, text := range texts {
			if found := tlsError.FindString(text); found != "" {
				return strings.Trim(strings.TrimSpace(found), `."'`)
			}
		}
	}
	return ""
}

// servingCertSentences say that the certificate the webhook's pods
// serve has already ended: "Its serving certificate in Secret
// policy-system/policy-tls expired at 2026-10-01 09:00." The date is a
// fact that already happened; nothing about the certificate's content is
// read.
func servingCertSentences(f caseFacts) []sentence {
	cause := f.p.Cause
	if !tlsCause(cause) || cause.ServingCert == nil {
		return nil
	}
	cert := cause.ServingCert
	text := "Its serving certificate in Secret " + cert.Secret.Namespace +
		"/" + cert.Secret.Name + " expired at " +
		cert.Expired.UTC().Format("2006-01-02 15:04")
	return []sentence{{part: partCause, text: endSentence(text)}}
}
