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
	subject := s.subject
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

// asciiSubject keeps only what SNS accepts in a subject: printable ASCII
// without line breaks, under 100 characters. The status emoji is dropped
// because SNS rejects it; the message body keeps it.
func asciiSubject(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= 0x20 && r <= 0x7e {
			b.WriteRune(r)
		}
	}
	subject := strings.TrimSpace(b.String())
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
