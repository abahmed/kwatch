package vonage

import (
	"net/url"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const vonageAPIURL = "https://rest.nexmo.com/sms/json"

type Vonage struct {
	url       string
	apiKey    string
	apiSecret string
	from      string
	to        string

	appCfg *config.App
}

// NewVonage returns a new Vonage object
func NewVonage(config map[string]interface{}, appCfg *config.App) *Vonage {
	apiKey, ok := config["apiKey"].(string)
	if !ok || len(apiKey) == 0 {
		klog.InfoS("initializing vonage with empty apiKey")
		return nil
	}

	apiSecret, ok := config["apiSecret"].(string)
	if !ok || len(apiSecret) == 0 {
		klog.InfoS("initializing vonage with empty apiSecret")
		return nil
	}

	from, ok := config["from"].(string)
	if !ok || len(from) == 0 {
		klog.InfoS("initializing vonage with empty from")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing vonage with empty to")
		return nil
	}

	klog.InfoS("initializing vonage", "from", from, "to", to)

	return &Vonage{
		url:       vonageAPIURL,
		apiKey:    apiKey,
		apiSecret: apiSecret,
		from:      from,
		to:        to,
		appCfg:    appCfg,
	}
}

// Name returns name of the provider
func (v *Vonage) Name() string {
	return "Vonage"
}

// SendEvent sends event to the provider
func (v *Vonage) SendEvent(e *event.Event) error {
	msg := e.FormatText(v.appCfg.ClusterName, "")
	return v.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (v *Vonage) SendMessage(msg string) error {
	form := url.Values{}
	form.Set("api_key", v.apiKey)
	form.Set("api_secret", v.apiSecret)
	form.Set("from", v.from)
	form.Set("to", v.to)
	form.Set("text", msg)

	_, err := util.Post(
		v.Name(), v.url, []byte(form.Encode()),
		"application/x-www-form-urlencoded", nil)
	return err
}
