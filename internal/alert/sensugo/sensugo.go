package sensugo

import (
	"encoding/json"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const sensuAPIPath = "/api/core/v2/namespaces/%s/events"

type sensuMetadata struct {
	Name string `json:"name"`
}

type sensuEntity struct {
	Metadata sensuMetadata `json:"metadata"`
}

type sensuCheck struct {
	Metadata sensuMetadata `json:"metadata"`
	Status   int           `json:"status"`
	Output   string        `json:"output"`
	Issued   int64         `json:"issued"`
}

type sensuPayload struct {
	Entity sensuEntity `json:"entity"`
	Check  sensuCheck  `json:"check"`
}

type Sensugo struct {
	url       string
	apiKey    string
	namespace string
	entity    string

	appCfg *config.App
}

// NewSensugo returns a new Sensugo object
func NewSensugo(config map[string]interface{}, appCfg *config.App) *Sensugo {
	url, ok := config["url"].(string)
	if !ok || len(url) == 0 {
		klog.InfoS("initializing sensugo with empty url")
		return nil
	}

	apiKey, ok := config["apiKey"].(string)
	if !ok || len(apiKey) == 0 {
		klog.InfoS("initializing sensugo with empty apiKey")
		return nil
	}

	namespace, _ := config["namespace"].(string)
	if len(namespace) == 0 {
		namespace = "default"
	}

	entity, _ := config["entity"].(string)
	if len(entity) == 0 {
		entity = "kwatch"
	}

	klog.InfoS("initializing sensugo", "url", url, "namespace", namespace)

	return &Sensugo{
		url:       strings.TrimRight(url, "/") + strings.Replace(sensuAPIPath, "%s", namespace, 1),
		apiKey:    apiKey,
		namespace: namespace,
		entity:    entity,
		appCfg:    appCfg,
	}
}

// Name returns name of the provider
func (s *Sensugo) Name() string {
	return "Sensu Go"
}

// SendEvent sends event to the provider
func (s *Sensugo) SendEvent(e *event.Event) error {
	msg := e.FormatText(s.appCfg.ClusterName, "")
	return s.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (s *Sensugo) SendMessage(msg string) error {
	payload := sensuPayload{
		Entity: sensuEntity{
			Metadata: sensuMetadata{Name: s.entity},
		},
		Check: sensuCheck{
			Metadata: sensuMetadata{Name: "kwatch"},
			Status:   1,
			Output:   msg,
			Issued:   time.Now().Unix(),
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(s.Name(), s.url, body, "application/json", map[string]string{
		"Authorization": "Key " + s.apiKey,
	})
	return err
}
