package pushover

import (
	"context"
	"net/url"
	"strconv"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
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

// SendEvent sends event to the provider
func (p *Pushover) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(p.clusterName, "")
	return p.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (p *Pushover) SendMessage(ctx context.Context, msg string) error {
	form := url.Values{}
	form.Set("token", p.token)
	form.Set("user", p.user)
	form.Set("message", msg)
	if len(p.title) > 0 {
		form.Set("title", p.title)
	}
	if p.priority != 0 {
		form.Set("priority", strconv.Itoa(p.priority))
	}
	if p.priority == 2 {
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
