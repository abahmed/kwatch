package plivo

import (
	"encoding/base64"
	"fmt"
	"net/url"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const plivoAPIURL = "https://api.plivo.com/v1/Account/%s/Message/"

type Plivo struct {
	url       string
	authID    string
	authToken string
	from      string
	to        string

	appCfg *config.App
}

// NewPlivo returns a new Plivo object
func NewPlivo(config map[string]interface{}, appCfg *config.App) *Plivo {
	authID, ok := config["authId"].(string)
	if !ok || len(authID) == 0 {
		klog.InfoS("initializing plivo with empty authId")
		return nil
	}

	authToken, ok := config["authToken"].(string)
	if !ok || len(authToken) == 0 {
		klog.InfoS("initializing plivo with empty authToken")
		return nil
	}

	from, ok := config["from"].(string)
	if !ok || len(from) == 0 {
		klog.InfoS("initializing plivo with empty from")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing plivo with empty to")
		return nil
	}

	klog.InfoS("initializing plivo", "from", from, "to", to)

	return &Plivo{
		url:       fmt.Sprintf(plivoAPIURL, authID),
		authID:    authID,
		authToken: authToken,
		from:      from,
		to:        to,
		appCfg:    appCfg,
	}
}

// Name returns name of the provider
func (p *Plivo) Name() string {
	return "Plivo"
}

// SendEvent sends event to the provider
func (p *Plivo) SendEvent(e *event.Event) error {
	msg := e.FormatText(p.appCfg.ClusterName, "")
	return p.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (p *Plivo) SendMessage(msg string) error {
	form := url.Values{}
	form.Set("src", p.from)
	form.Set("dst", p.to)
	form.Set("text", msg)

	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(p.authID+":"+p.authToken))

	_, err := util.Post(
		p.Name(), p.url, []byte(form.Encode()),
		"application/x-www-form-urlencoded",
		map[string]string{
			"Authorization": auth,
		})
	return err
}
