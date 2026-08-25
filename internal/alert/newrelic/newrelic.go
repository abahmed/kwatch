package newrelic

import (
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const newRelicAPIURL = "https://insights-collector.newrelic.com/v1/accounts/%s/events"

type NewRelic struct {
	url       string
	apiKey    string
	accountID string

	appCfg *config.App
}

// NewNewRelic returns a new NewRelic object
func NewNewRelic(config map[string]interface{}, appCfg *config.App) *NewRelic {
	apiKey, ok := config["apiKey"].(string)
	if !ok || len(apiKey) == 0 {
		klog.InfoS("initializing newrelic with empty apiKey")
		return nil
	}

	accountID, ok := config["accountId"].(string)
	if !ok || len(accountID) == 0 {
		klog.InfoS("initializing newrelic with empty accountId")
		return nil
	}

	klog.InfoS("initializing newrelic", "accountId", accountID)

	return &NewRelic{
		url:       fmt.Sprintf(newRelicAPIURL, accountID),
		apiKey:    apiKey,
		accountID: accountID,
		appCfg:    appCfg,
	}
}

// Name returns name of the provider
func (n *NewRelic) Name() string {
	return "New Relic"
}

// SendEvent sends event to the provider
func (n *NewRelic) SendEvent(e *event.Event) error {
	return n.SendMessage(e.FormatText(n.appCfg.ClusterName, ""))
}

// SendMessage sends text message to the provider
func (n *NewRelic) SendMessage(msg string) error {
	eventPayload := map[string]interface{}{
		"eventType": "KwatchAlert",
		"cluster":   n.appCfg.ClusterName,
		"message":   msg,
	}

	body, err := json.Marshal([]interface{}{eventPayload})
	if err != nil {
		return err
	}

	_, err = util.Post(n.Name(), n.url, body, "application/json", map[string]string{
		"Api-Key": n.apiKey,
	})
	return err
}
