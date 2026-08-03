package twilio

import (
	"encoding/base64"
	"fmt"
	"net/url"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const twilioAPIURL = "https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json"

type Twilio struct {
	url        string
	accountSID string
	authToken  string
	from       string
	to         string

	appCfg *config.App
}

// NewTwilio returns a new Twilio object
func NewTwilio(config map[string]interface{}, appCfg *config.App) *Twilio {
	accountSID, ok := config["accountSid"].(string)
	if !ok || len(accountSID) == 0 {
		klog.InfoS("initializing twilio with empty accountSid")
		return nil
	}

	authToken, ok := config["authToken"].(string)
	if !ok || len(authToken) == 0 {
		klog.InfoS("initializing twilio with empty authToken")
		return nil
	}

	from, ok := config["from"].(string)
	if !ok || len(from) == 0 {
		klog.InfoS("initializing twilio with empty from")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing twilio with empty to")
		return nil
	}

	klog.InfoS("initializing twilio", "from", from, "to", to)

	return &Twilio{
		url:        fmt.Sprintf(twilioAPIURL, accountSID),
		accountSID: accountSID,
		authToken:  authToken,
		from:       from,
		to:         to,
		appCfg:     appCfg,
	}
}

// Name returns name of the provider
func (t *Twilio) Name() string {
	return "Twilio"
}

// SendEvent sends event to the provider
func (t *Twilio) SendEvent(e *event.Event) error {
	msg := e.FormatText(t.appCfg.ClusterName, "")
	return t.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (t *Twilio) SendMessage(msg string) error {
	form := url.Values{}
	form.Set("From", t.from)
	form.Set("To", t.to)
	form.Set("Body", msg)

	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(t.accountSID+":"+t.authToken))

	_, err := util.Post(
		t.Name(), t.url, []byte(form.Encode()),
		"application/x-www-form-urlencoded",
		map[string]string{
			"Authorization": auth,
		})
	return err
}
