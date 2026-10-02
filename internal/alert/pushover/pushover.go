package pushover

import (
	"context"
	"net/url"
	"strconv"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const pushoverAPIURL = "https://api.pushover.net/1/messages.json"

type Pushover struct {
	sender   transport.Sender
	url      string
	token    string
	user     string
	title    string
	priority int
	retry    int
	expire   int

	clusterName string
}

// NewPushover returns a new Pushover object

func NewPushover(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Pushover {
	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing pushover with empty token")
		return nil
	}

	user, ok := config["user"].(string)
	if !ok || len(user) == 0 {
		klog.InfoS("initializing pushover with empty user")
		return nil
	}

	title, _ := config["title"].(string)

	priority := 0
	switch v := config["priority"].(type) {
	case float64:
		priority = int(v)
	case int:
		priority = v
	case int64:
		priority = int(v)
	}
	if priority < -2 || priority > 2 {
		klog.InfoS("initializing pushover with invalid priority",
			"priority", priority)
		return nil
	}
	retry := integerSetting(config["retry"])
	expire := integerSetting(config["expire"])
	if priority == 2 && (retry < 30 || expire < 1 || expire > 10800) {
		klog.InfoS(
			"pushover emergency priority requires valid retry and expire",
			"retry", retry, "expire", expire,
		)
		return nil
	}

	klog.InfoS("initializing pushover", "title", title)

	return &Pushover{
		sender:      transport.NewSender(dependencies),
		url:         pushoverAPIURL,
		token:       token,
		user:        user,
		title:       title,
		priority:    priority,
		retry:       retry,
		expire:      expire,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (p *Pushover) Name() string {
	return "Pushover"
}

// SendIncident sends the incident's one-line lead as the push message.
// Open incidents use the configured priority; a resolve is sent at
// normal priority so it never repeats an emergency alarm.
func (p *Pushover) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	priority := p.priority
	if m.Resolved() {
		priority = 0
	}
	return p.send(ctx, m.ShortText(), priority)
}

// SendMessage sends text message to the provider
func (p *Pushover) SendMessage(ctx context.Context, msg string) error {
	return p.send(ctx, msg, p.priority)
}

// send posts one message. Emergency priority 2 always carries the retry
// and expire values validated at construction.
func (p *Pushover) send(
	ctx context.Context, msg string, priority int,
) error {
	form := url.Values{}
	form.Set("token", p.token)
	form.Set("user", p.user)
	form.Set("message", msg)
	if len(p.title) > 0 {
		form.Set("title", p.title)
	}
	if priority != 0 {
		form.Set("priority", strconv.Itoa(priority))
	}
	if priority == 2 {
		form.Set("retry", strconv.Itoa(p.retry))
		form.Set("expire", strconv.Itoa(p.expire))
	}

	_, err := p.sender.Send(ctx, transport.Request{
		Provider: p.Name(), URL: p.url, Body: []byte(form.Encode()),
		ContentType: "application/x-www-form-urlencoded",
	})
	return err
}

func integerSetting(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}
