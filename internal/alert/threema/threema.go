package threema

import (
	"net/url"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const threemaAPIURL = "https://gateway.threema.ch/push_simple"

type Threema struct {
	url       string
	gatewayID string
	secret    string
	to        string

	appCfg *config.App
}

// NewThreema returns a new Threema object
func NewThreema(config map[string]interface{}, appCfg *config.App) *Threema {
	gatewayID, ok := config["gatewayId"].(string)
	if !ok || len(gatewayID) == 0 {
		klog.InfoS("initializing threema with empty gatewayId")
		return nil
	}

	secret, ok := config["secret"].(string)
	if !ok || len(secret) == 0 {
		klog.InfoS("initializing threema with empty secret")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing threema with empty to")
		return nil
	}

	klog.InfoS("initializing threema", "gatewayId", gatewayID)

	return &Threema{
		url:       threemaAPIURL,
		gatewayID: gatewayID,
		secret:    secret,
		to:        to,
		appCfg:    appCfg,
	}
}

// Name returns name of the provider
func (s *Threema) Name() string {
	return "Threema"
}

// SendEvent sends event to the provider
func (s *Threema) SendEvent(e *event.Event) error {
	msg := e.FormatText(s.appCfg.ClusterName, "")
	return s.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (s *Threema) SendMessage(msg string) error {
	form := url.Values{}
	form.Set("from", s.gatewayID)
	form.Set("to", s.to)
	form.Set("secret", s.secret)
	form.Set("text", msg)

	body := []byte(form.Encode())
	_, err := util.Post(s.Name(), s.url, body, "application/x-www-form-urlencoded", nil)
	return err
}
