package messagebird

import (
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const messagebirdAPIURL = "https://rest.messagebird.com/messages"

type messagebirdPayload struct {
	Originator string   `json:"originator"`
	Recipients []string `json:"recipients"`
	Body       string   `json:"body"`
}

type Messagebird struct {
	url       string
	accessKey string
	from      string
	to        string

	appCfg *config.App
}

// NewMessagebird returns a new Messagebird object
func NewMessagebird(config map[string]interface{}, appCfg *config.App) *Messagebird {
	accessKey, ok := config["accessKey"].(string)
	if !ok || len(accessKey) == 0 {
		klog.InfoS("initializing messagebird with empty accessKey")
		return nil
	}

	from, ok := config["from"].(string)
	if !ok || len(from) == 0 {
		klog.InfoS("initializing messagebird with empty from")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing messagebird with empty to")
		return nil
	}

	klog.InfoS("initializing messagebird", "from", from, "to", to)

	return &Messagebird{
		url:       messagebirdAPIURL,
		accessKey: accessKey,
		from:      from,
		to:        to,
		appCfg:    appCfg,
	}
}

// Name returns name of the provider
func (m *Messagebird) Name() string {
	return "Messagebird"
}

// SendEvent sends event to the provider
func (m *Messagebird) SendEvent(e *event.Event) error {
	msg := e.FormatText(m.appCfg.ClusterName, "")
	return m.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (m *Messagebird) SendMessage(msg string) error {
	payload := messagebirdPayload{
		Originator: m.from,
		Recipients: []string{m.to},
		Body:       msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(m.Name(), m.url, body, "application/json", map[string]string{
		"Authorization": "AccessKey " + m.accessKey,
	})
	return err
}
