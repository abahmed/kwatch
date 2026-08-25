package ses

import (
	"fmt"
	"net/url"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const (
	sesServiceName   = "ses"
	sesURLFormat     = "https://email.%s.amazonaws.com/"
	defaultSESRegion = "us-east-1"
)

type Ses struct {
	url             string
	region          string
	accessKeyID     string
	secretAccessKey string
	from            string
	to              []string
	subject         string

	appCfg *config.App
}

// NewSes returns a new Ses object
func NewSes(config map[string]interface{}, appCfg *config.App) *Ses {
	accessKeyID, ok := config["accessKeyId"].(string)
	if !ok || len(accessKeyID) == 0 {
		klog.InfoS("initializing ses with empty accessKeyId")
		return nil
	}

	secretAccessKey, ok := config["secretAccessKey"].(string)
	if !ok || len(secretAccessKey) == 0 {
		klog.InfoS("initializing ses with empty secretAccessKey")
		return nil
	}

	from, ok := config["from"].(string)
	if !ok || len(from) == 0 {
		klog.InfoS("initializing ses with empty from")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing ses with empty to")
		return nil
	}

	var recipients []string
	for _, t := range strings.Split(to, ",") {
		if r := strings.TrimSpace(t); len(r) > 0 {
			recipients = append(recipients, r)
		}
	}
	if len(recipients) == 0 {
		klog.InfoS("initializing ses with empty to")
		return nil
	}

	region := defaultSESRegion
	if r, ok := config["region"].(string); ok && len(r) > 0 {
		region = r
	}

	subject, _ := config["subject"].(string)

	klog.InfoS("initializing ses", "region", region, "from", from)

	return &Ses{
		url:             fmt.Sprintf(sesURLFormat, region),
		region:          region,
		accessKeyID:     accessKeyID,
		secretAccessKey: secretAccessKey,
		from:            from,
		to:              recipients,
		subject:         subject,
		appCfg:          appCfg,
	}
}

// Name returns name of the provider
func (s *Ses) Name() string {
	return "SES"
}

// SendEvent sends event to the provider
func (s *Ses) SendEvent(e *event.Event) error {
	msg := e.FormatText(s.appCfg.ClusterName, "")
	return s.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (s *Ses) SendMessage(msg string) error {
	subject := s.subject
	if len(subject) == 0 {
		subject = "kwatch alert"
	}

	form := url.Values{}
	form.Set("Action", "SendEmail")
	form.Set("Version", "2010-12-01")
	form.Set("Source", s.from)
	for i, r := range s.to {
		form.Set(fmt.Sprintf("Destination.ToAddresses.member.%d", i+1), r)
	}
	form.Set("Message.Subject.Data", subject)
	form.Set("Message.Subject.Charset", "UTF-8")
	form.Set("Message.Body.Text.Data", msg)
	form.Set("Message.Body.Text.Charset", "UTF-8")

	body := []byte(form.Encode())
	contentType := "application/x-www-form-urlencoded"

	headers, err := util.SignAWSV4(
		s.accessKeyID, s.secretAccessKey, s.region, sesServiceName,
		"POST", s.url, body)
	if err != nil {
		return err
	}

	_, err = util.Post(s.Name(), s.url, body, contentType, headers)
	return err
}
