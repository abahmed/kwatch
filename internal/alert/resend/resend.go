package resend

import (
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const resendAPIURL = "https://api.resend.com/emails"

type Resend struct {
	url     string
	apiKey  string
	from    string
	to      []string
	subject string

	appCfg *config.App
}

// NewResend returns a new Resend object
func NewResend(config map[string]interface{}, appCfg *config.App) *Resend {
	apiKey, ok := config["apiKey"].(string)
	if !ok || len(apiKey) == 0 {
		klog.InfoS("initializing resend with empty apiKey")
		return nil
	}

	from, ok := config["from"].(string)
	if !ok || len(from) == 0 {
		klog.InfoS("initializing resend with empty from")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing resend with empty to")
		return nil
	}

	var recipients []string
	for _, t := range strings.Split(to, ",") {
		if r := strings.TrimSpace(t); len(r) > 0 {
			recipients = append(recipients, r)
		}
	}
	if len(recipients) == 0 {
		klog.InfoS("initializing resend with empty to")
		return nil
	}

	subject, _ := config["subject"].(string)

	klog.InfoS("initializing resend", "from", from)

	return &Resend{
		url:     resendAPIURL,
		apiKey:  apiKey,
		from:    from,
		to:      recipients,
		subject: subject,
		appCfg:  appCfg,
	}
}

// Name returns name of the provider
func (s *Resend) Name() string {
	return "Resend"
}

// SendEvent sends event to the provider
func (s *Resend) SendEvent(e *event.Event) error {
	msg := e.FormatText(s.appCfg.ClusterName, "")
	return s.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (s *Resend) SendMessage(msg string) error {
	subject := s.subject
	if len(subject) == 0 {
		subject = "kwatch alert"
	}

	payload := map[string]interface{}{
		"from":    s.from,
		"to":      s.to,
		"subject": subject,
		"text":    msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(s.Name(), s.url, body, "application/json", map[string]string{
		"Authorization": "Bearer " + s.apiKey,
	})
	return err
}
