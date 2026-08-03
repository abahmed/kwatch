package splunk

import (
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

type splunkPayload struct {
	Event      map[string]interface{} `json:"event"`
	Source     string                 `json:"source,omitempty"`
	Sourcetype string                 `json:"sourcetype,omitempty"`
	Index      string                 `json:"index,omitempty"`
	Host       string                 `json:"host,omitempty"`
}

type Splunk struct {
	url        string
	token      string
	source     string
	sourcetype string
	index      string
	host       string

	appCfg *config.App
}

// NewSplunk returns a new Splunk object
func NewSplunk(config map[string]interface{}, appCfg *config.App) *Splunk {
	url, ok := config["url"].(string)
	if !ok || len(url) == 0 {
		klog.InfoS("initializing splunk with empty url")
		return nil
	}

	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing splunk with empty token")
		return nil
	}

	source, _ := config["source"].(string)
	sourcetype, _ := config["sourcetype"].(string)
	index, _ := config["index"].(string)
	host, _ := config["host"].(string)

	klog.InfoS("initializing splunk", "url", url, "source", source)

	return &Splunk{
		url:        url,
		token:      token,
		source:     source,
		sourcetype: sourcetype,
		index:      index,
		host:       host,
		appCfg:     appCfg,
	}
}

// Name returns name of the provider
func (s *Splunk) Name() string {
	return "Splunk"
}

// SendEvent sends event to the provider
func (s *Splunk) SendEvent(e *event.Event) error {
	return s.SendMessage(e.FormatText(s.appCfg.ClusterName, ""))
}

// SendMessage sends text message to the provider
func (s *Splunk) SendMessage(msg string) error {
	payload := splunkPayload{
		Event: map[string]interface{}{
			"message": msg,
			"source":  "kwatch",
		},
		Source:     s.source,
		Sourcetype: s.sourcetype,
		Index:      s.index,
		Host:       s.host,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(s.Name(), s.url, body, "application/json", map[string]string{
		"Authorization": "Splunk " + s.token,
	})
	return err
}
