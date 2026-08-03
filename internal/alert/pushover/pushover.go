package pushover

import (
	"net/url"
	"strconv"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const pushoverAPIURL = "https://api.pushover.net/1/messages.json"

type Pushover struct {
	url      string
	token    string
	user     string
	title    string
	priority int

	appCfg *config.App
}

// NewPushover returns a new Pushover object
func NewPushover(config map[string]interface{}, appCfg *config.App) *Pushover {
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

	klog.InfoS("initializing pushover", "user", user, "title", title)

	return &Pushover{
		url:      pushoverAPIURL,
		token:    token,
		user:     user,
		title:    title,
		priority: priority,
		appCfg:   appCfg,
	}
}

// Name returns name of the provider
func (p *Pushover) Name() string {
	return "Pushover"
}

// SendEvent sends event to the provider
func (p *Pushover) SendEvent(e *event.Event) error {
	msg := e.FormatText(p.appCfg.ClusterName, "")
	return p.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (p *Pushover) SendMessage(msg string) error {
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

	_, err := util.Post(
		p.Name(), p.url, []byte(form.Encode()),
		"application/x-www-form-urlencoded", nil)
	return err
}
