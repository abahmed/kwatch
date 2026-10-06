package pushover

import (
	"context"
	"net/url"
	"strconv"
	"sync"

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

	// receipts maps a conversation key to the emergency receipt its
	// announcement started, so the resolve can cancel the alarm.
	mu       sync.Mutex
	receipts map[string]string
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
		receipts:    map[string]string{},
	}
}

// Name returns name of the provider
func (p *Pushover) Name() string {
	return "Pushover"
}

// SendIncident sends the incident's one-line lead as the push message.
// Only the announcement of a page-tier incident uses the configured
// priority, so an emergency alarm starts once per incident. Updates are
// normal priority, and the resolve cancels the alarm the announcement
// started, then sends the "resolved" push (see receipts.go).
func (p *Pushover) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	if m.Resolved() {
		// Cancel first: a transient cancel failure retries before
		// anything was sent, so "resolved" is never pushed twice. A
		// cancelled receipt is forgotten, so the retry skips it.
		if err := p.cancelReceipt(ctx, m.Key); err != nil {
			return err
		}
		return p.send(ctx, m.ShortText(), 0, "")
	}
	priority := p.priorityFor(m)
	return p.send(ctx, m.ShortText(), priority, m.Key)
}

// priorityFor picks the priority of a live (not resolved) message.
// Summaries, digests, notices and updates are normal priority; only an
// opening message uses the configured one, and emergency (2) only when
// the incident is page-tier. A lower tier is held at high (1).
func (p *Pushover) priorityFor(m notification.Message) int {
	if m.IsInformational() || !m.IsOpening() {
		return 0
	}
	if p.priority == 2 && m.Status != notification.StatusCritical {
		return 1
	}
	return p.priority
}

// SendMessage sends a plain operator message at normal priority.
func (p *Pushover) SendMessage(ctx context.Context, msg string) error {
	return p.send(ctx, msg, 0, "")
}

// send posts one message. Emergency priority 2 always carries the retry
// and expire values validated at construction. When it starts an
// emergency for key, the receipt in the answer is remembered.
func (p *Pushover) send(
	ctx context.Context, msg string, priority int, key string,
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

	body, err := p.sender.Send(ctx, transport.Request{
		Provider: p.Name(), URL: p.url, Body: []byte(form.Encode()),
		ContentType: "application/x-www-form-urlencoded",
	})
	if err == nil && priority == 2 && key != "" {
		p.rememberReceipt(key, body)
	}
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
