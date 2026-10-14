package sns

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/signing"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const (
	snsServiceName   = "sns"
	snsURLFormat     = "https://sns.%s.amazonaws.com/"
	defaultSNSRegion = "us-east-1"
)

type Sns struct {
	sender          transport.Sender
	url             string
	region          string
	accessKeyID     string
	sessionToken    string
	secretAccessKey string
	topicArn        string
	targetArn       string
	subject         string
	now             func() time.Time

	clusterName string
}

// NewSns returns a new Sns object

func NewSns(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Sns {
	sessionToken, _ := config["sessionToken"].(string)
	accessKeyID, ok := config["accessKeyId"].(string)
	if !ok || len(accessKeyID) == 0 {
		klog.InfoS("initializing sns with empty accessKeyId")
		return nil
	}

	secretAccessKey, ok := config["secretAccessKey"].(string)
	if !ok || len(secretAccessKey) == 0 {
		klog.InfoS("initializing sns with empty secretAccessKey")
		return nil
	}

	topicArn, _ := config["topicArn"].(string)
	targetArn, _ := config["targetArn"].(string)
	if len(topicArn) == 0 && len(targetArn) == 0 {
		klog.InfoS("initializing sns with empty topicArn or targetArn")
		return nil
	}

	region := defaultSNSRegion
	if r, ok := config["region"].(string); ok && len(r) > 0 {
		region = r
	}

	subject, _ := config["subject"].(string)

	klog.InfoS("initializing sns", "region", region, "topicArn", topicArn)

	return &Sns{
		sender:          transport.NewSender(dependencies),
		url:             fmt.Sprintf(snsURLFormat, region),
		region:          region,
		accessKeyID:     accessKeyID,
		sessionToken:    sessionToken,
		secretAccessKey: secretAccessKey,
		topicArn:        topicArn,
		targetArn:       targetArn,
		subject:         subject,
		clusterName:     clusterName,
		now:             dependencies.Now,
	}
}

// Name returns name of the provider
func (s *Sns) Name() string {
	return "SNS"
}

// maxSubjectBytes is SNS's limit: the subject must be shorter than 100
// characters.
const maxSubjectBytes = 99

// SendIncident publishes the narrative with the lead as the subject. The
// incident key and status travel as message attributes, so subscribers
// can filter and group one incident's messages.
func (s *Sns) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	subject := asciiSubject(s.subject)
	if len(subject) == 0 {
		subject = asciiSubject(m.ShortText())
	}
	message := m.NoteText()
	if len(m.Output) > 0 {
		message += "\n\nLast output:\n" + strings.Join(m.Output, "\n")
	}
	form := s.publishForm(message, subject)
	setAttribute(form, 1, "kwatch.key", m.AlertKey(s.clusterName))
	setAttribute(form, 2, "kwatch.status", m.Status.String())
	return s.publish(ctx, form)
}

// subjectReplacer turns the Latin letters and typographic punctuation
// that have a plain ASCII form into it, so a subject keeps its words.
// Everything else outside ASCII (other scripts, emoji) has no trivial
// form and is dropped.
var subjectReplacer = strings.NewReplacer(
	"à", "a", "á", "a", "â", "a", "ã", "a", "ä", "a", "å", "a", "ā", "a",
	"À", "A", "Á", "A", "Â", "A", "Ã", "A", "Ä", "A", "Å", "A", "Ā", "A",
	"ç", "c", "Ç", "C", "ć", "c", "č", "c", "Č", "C",
	"è", "e", "é", "e", "ê", "e", "ë", "e", "ē", "e", "ę", "e",
	"È", "E", "É", "E", "Ê", "E", "Ë", "E", "Ē", "E",
	"ì", "i", "í", "i", "î", "i", "ï", "i", "ī", "i",
	"Ì", "I", "Í", "I", "Î", "I", "Ï", "I", "Ī", "I",
	"ñ", "n", "Ñ", "N", "ń", "n", "ł", "l", "Ł", "L",
	"ò", "o", "ó", "o", "ô", "o", "õ", "o", "ö", "o", "ø", "o", "ō", "o",
	"Ò", "O", "Ó", "O", "Ô", "O", "Õ", "O", "Ö", "O", "Ø", "O", "Ō", "O",
	"ś", "s", "š", "s", "Š", "S", "ß", "ss",
	"ù", "u", "ú", "u", "û", "u", "ü", "u", "ū", "u",
	"Ù", "U", "Ú", "U", "Û", "U", "Ü", "U", "Ū", "U",
	"ý", "y", "ÿ", "y", "Ý", "Y", "ź", "z", "ż", "z", "ž", "z", "Ž", "Z",
	"æ", "ae", "Æ", "AE", "œ", "oe", "Œ", "OE",
	"‘", "'", "’", "'", "“", `"`, "”", `"`, "«", `"`, "»", `"`,
	"–", "-", "—", "-", "…", "...", "\u00a0", " ",
)

// asciiSubject makes value a subject SNS accepts: printable ASCII on one
// line, shorter than 100 characters, starting with a letter, number or
// punctuation mark. The status emoji and other characters with no ASCII
// form are dropped (the message body keeps them), and the whitespace
// around a dropped word collapses, so no gap is left.
func asciiSubject(value string) string {
	var b strings.Builder
	// dropped is set when a character was dropped since the last one
	// written, so a pair of quotes that is empty only because of the
	// drop can be removed while an empty pair in the subject stays.
	dropped := false
	for _, r := range subjectReplacer.Replace(value) {
		switch {
		case r == '"' && dropped && strings.HasSuffix(b.String(), `"`):
			text := strings.TrimSuffix(b.String(), `"`)
			b.Reset()
			b.WriteString(text)
			dropped = false
		case r >= 0x21 && r <= 0x7e:
			b.WriteRune(r)
			dropped = false
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			b.WriteByte(' ')
		default:
			dropped = true
		}
	}
	text := b.String()
	subject := strings.Join(strings.Fields(text), " ")
	if len(subject) > maxSubjectBytes {
		subject = strings.TrimSpace(subject[:maxSubjectBytes])
	}
	return subject
}

func setAttribute(form url.Values, index int, name, value string) {
	prefix := fmt.Sprintf("MessageAttributes.entry.%d.", index)
	form.Set(prefix+"Name", name)
	form.Set(prefix+"Value.DataType", "String")
	form.Set(prefix+"Value.StringValue", value)
}

// SendMessage sends text message to the provider
func (s *Sns) SendMessage(ctx context.Context, msg string) error {
	return s.publish(ctx, s.publishForm(msg, s.subject))
}

func (s *Sns) publishForm(message, subject string) url.Values {
	form := url.Values{}
	form.Set("Action", "Publish")
	form.Set("Version", "2010-03-31")
	if len(s.targetArn) > 0 {
		form.Set("TargetArn", s.targetArn)
	} else {
		form.Set("TopicArn", s.topicArn)
	}
	form.Set("Message", message)
	if len(subject) > 0 {
		form.Set("Subject", subject)
	}
	return form
}

// publish signs and sends one SNS Publish request.
func (s *Sns) publish(ctx context.Context, form url.Values) error {
	body := []byte(form.Encode())
	contentType := "application/x-www-form-urlencoded"

	headers, err := signing.SignAWSV4At(
		signing.Credentials{
			AccessKeyID: s.accessKeyID, SecretAccessKey: s.secretAccessKey,
			SessionToken: s.sessionToken,
		}, s.region, snsServiceName,
		"POST", s.url, body, s.now())
	if err != nil {
		return err
	}

	_, err = s.sender.Send(ctx, transport.Request{
		Provider: s.Name(), URL: s.url, Body: body,
		ContentType: contentType, Headers: headers,
	})
	return err
}
