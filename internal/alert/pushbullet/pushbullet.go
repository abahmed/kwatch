package pushbullet

import (
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const pushbulletAPIURL = "https://api.pushbullet.com/v2/pushes"

type pushbulletPayload struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

type Pushbullet struct {
	url         string
	accessToken string

	appCfg *config.App
}

// NewPushbullet returns a new Pushbullet object
func NewPushbullet(config map[string]interface{}, appCfg *config.App) *Pushbullet {
	accessToken, ok := config["accessToken"].(string)
	if !ok || len(accessToken) == 0 {
		klog.InfoS("initializing pushbullet with empty accessToken")
		return nil
	}

	klog.InfoS("initializing pushbullet")

	return &Pushbullet{
		url:         pushbulletAPIURL,
		accessToken: accessToken,
		appCfg:      appCfg,
	}
}

// Name returns name of the provider
func (s *Pushbullet) Name() string {
	return "Pushbullet"
}

// SendEvent sends event to the provider
func (s *Pushbullet) SendEvent(e *event.Event) error {
	msg := e.FormatText(s.appCfg.ClusterName, "")
	return s.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (s *Pushbullet) SendMessage(msg string) error {
	title := "kwatch alert"
	if len(s.appCfg.ClusterName) > 0 {
		title = "kwatch alert: " + s.appCfg.ClusterName
	}

	payload := pushbulletPayload{
		Type:  "note",
		Title: title,
		Body:  msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(s.Name(), s.url, body, "application/json", map[string]string{
		"Access-Token": s.accessToken,
	})
	return err
}
