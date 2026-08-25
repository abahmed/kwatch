package zulip

import (
	"encoding/base64"
	"net/url"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const zulipAPIURL = "https://api.zulip.com"

type Zulip struct {
	url     string
	email   string
	token   string
	channel string
	title   string

	appCfg *config.App
}

// NewZulip returns a new Zulip object
func NewZulip(config map[string]interface{}, appCfg *config.App) *Zulip {
	email, ok := config["email"].(string)
	if !ok || len(email) == 0 {
		klog.InfoS("initializing zulip with empty email")
		return nil
	}

	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing zulip with empty token")
		return nil
	}

	channel, ok := config["channel"].(string)
	if !ok || len(channel) == 0 {
		klog.InfoS("initializing zulip with empty channel")
		return nil
	}

	server := zulipAPIURL
	if s, ok := config["url"].(string); ok && len(s) > 0 {
		server = s
	}

	title, _ := config["title"].(string)

	klog.InfoS("initializing zulip", "url", server, "channel", channel)

	return &Zulip{
		url:     strings.TrimRight(server, "/") + "/api/v1/messages",
		email:   email,
		token:   token,
		channel: channel,
		title:   title,
		appCfg:  appCfg,
	}
}

// Name returns name of the provider
func (z *Zulip) Name() string {
	return "Zulip"
}

// SendEvent sends event to the provider
func (z *Zulip) SendEvent(e *event.Event) error {
	msg := e.FormatText(z.appCfg.ClusterName, "")
	return z.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (z *Zulip) SendMessage(msg string) error {
	subject := z.title
	if len(subject) == 0 {
		subject = "kwatch alert"
	}

	form := url.Values{}
	form.Set("type", "stream")
	form.Set("to", z.channel)
	form.Set("subject", subject)
	form.Set("content", msg)

	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(z.email+":"+z.token))

	_, err := util.Post(
		z.Name(), z.url, []byte(form.Encode()),
		"application/x-www-form-urlencoded",
		map[string]string{
			"Authorization": auth,
		})
	return err
}
