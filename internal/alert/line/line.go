package line

import (
	"net/url"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const lineAPIURL = "https://notify-api.line.me/api/notify"

type Line struct {
	url   string
	token string

	appCfg *config.App
}

// NewLine returns a new Line object
func NewLine(config map[string]interface{}, appCfg *config.App) *Line {
	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing line with empty token")
		return nil
	}

	klog.InfoS("initializing line")

	return &Line{
		url:    lineAPIURL,
		token:  token,
		appCfg: appCfg,
	}
}

// Name returns name of the provider
func (l *Line) Name() string {
	return "Line"
}

// SendEvent sends event to the provider
func (l *Line) SendEvent(e *event.Event) error {
	msg := e.FormatText(l.appCfg.ClusterName, "")
	return l.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (l *Line) SendMessage(msg string) error {
	form := url.Values{}
	form.Set("message", msg)

	_, err := util.Post(
		l.Name(), l.url, []byte(form.Encode()),
		"application/x-www-form-urlencoded",
		map[string]string{
			"Authorization": "Bearer " + l.token,
		})
	return err
}
