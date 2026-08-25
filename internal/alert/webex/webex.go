package webex

import (
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const webexAPIURL = "https://webexapis.com/v1/messages"

type webexPayload struct {
	RoomID        string `json:"roomId,omitempty"`
	ToPersonEmail string `json:"toPersonEmail,omitempty"`
	Markdown      string `json:"markdown,omitempty"`
	Text          string `json:"text,omitempty"`
}

type Webex struct {
	url           string
	accessToken   string
	roomID        string
	toPersonEmail string

	appCfg *config.App
}

// NewWebex returns a new Webex object
func NewWebex(config map[string]interface{}, appCfg *config.App) *Webex {
	accessToken, ok := config["accessToken"].(string)
	if !ok || len(accessToken) == 0 {
		klog.InfoS("initializing webex with empty accessToken")
		return nil
	}

	roomID, _ := config["roomId"].(string)
	toPersonEmail, _ := config["toPersonEmail"].(string)
	if len(roomID) == 0 && len(toPersonEmail) == 0 {
		klog.InfoS("initializing webex with empty roomId and toPersonEmail")
		return nil
	}

	klog.InfoS("initializing webex", "roomId", roomID, "toPersonEmail", toPersonEmail)

	return &Webex{
		url:           webexAPIURL,
		accessToken:   accessToken,
		roomID:        roomID,
		toPersonEmail: toPersonEmail,
		appCfg:        appCfg,
	}
}

// Name returns name of the provider
func (w *Webex) Name() string {
	return "Webex"
}

// SendEvent sends event to the provider
func (w *Webex) SendEvent(e *event.Event) error {
	msg := e.FormatMarkdown(w.appCfg.ClusterName, "", "\n\n")
	return w.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (w *Webex) SendMessage(msg string) error {
	payload := webexPayload{
		RoomID:        w.roomID,
		ToPersonEmail: w.toPersonEmail,
		Markdown:      msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(w.Name(), w.url, body, "application/json", map[string]string{
		"Authorization": "Bearer " + w.accessToken,
	})
	return err
}
