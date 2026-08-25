package mailgun

import (
	"encoding/base64"
	"net/url"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const mailgunAPIURL = "https://api.mailgun.net/v3"

type Mailgun struct {
	url     string
	apiKey  string
	domain  string
	from    string
	to      []string
	subject string

	appCfg *config.App
}

// NewMailgun returns a new Mailgun object
func NewMailgun(config map[string]interface{}, appCfg *config.App) *Mailgun {
	apiKey, ok := config["apiKey"].(string)
	if !ok || len(apiKey) == 0 {
		klog.InfoS("initializing mailgun with empty apiKey")
		return nil
	}

	domain, ok := config["domain"].(string)
	if !ok || len(domain) == 0 {
		klog.InfoS("initializing mailgun with empty domain")
		return nil
	}

	from, ok := config["from"].(string)
	if !ok || len(from) == 0 {
		klog.InfoS("initializing mailgun with empty from")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing mailgun with empty to")
		return nil
	}

	var recipients []string
	for _, t := range strings.Split(to, ",") {
		if r := strings.TrimSpace(t); len(r) > 0 {
			recipients = append(recipients, r)
		}
	}
	if len(recipients) == 0 {
		klog.InfoS("initializing mailgun with empty to")
		return nil
	}

	subject, _ := config["subject"].(string)

	server := mailgunAPIURL
	if u, ok := config["url"].(string); ok && len(u) > 0 {
		server = u
	}

	klog.InfoS("initializing mailgun", "domain", domain, "from", from)

	return &Mailgun{
		url:     strings.TrimRight(server, "/") + "/" + domain + "/messages",
		apiKey:  apiKey,
		domain:  domain,
		from:    from,
		to:      recipients,
		subject: subject,
		appCfg:  appCfg,
	}
}

// Name returns name of the provider
func (s *Mailgun) Name() string {
	return "Mailgun"
}

// SendEvent sends event to the provider
func (s *Mailgun) SendEvent(e *event.Event) error {
	msg := e.FormatText(s.appCfg.ClusterName, "")
	return s.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (s *Mailgun) SendMessage(msg string) error {
	subject := s.subject
	if len(subject) == 0 {
		subject = "kwatch alert"
	}

	form := url.Values{}
	form.Set("from", s.from)
	for _, t := range s.to {
		form.Add("to", t)
	}
	form.Set("subject", subject)
	form.Set("text", msg)

	body := []byte(form.Encode())

	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte("api:"+s.apiKey))
	_, err := util.Post(s.Name(), s.url, body, "application/x-www-form-urlencoded", map[string]string{
		"Authorization": auth,
	})
	return err
}
