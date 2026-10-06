package splunk

import (
	"context"
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/structured"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

type splunkPayload struct {
	Event      map[string]interface{} `json:"event"`
	Source     string                 `json:"source,omitempty"`
	Sourcetype string                 `json:"sourcetype,omitempty"`
	Index      string                 `json:"index,omitempty"`
	Host       string                 `json:"host,omitempty"`
}

type Splunk struct {
	sender     transport.Sender
	url        string
	token      string
	source     string
	sourcetype string
	index      string
	host       string

	clusterName string
}

// NewSplunk returns a new Splunk object

func NewSplunk(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Splunk {
	url, ok := config["url"].(string)
	if !ok || len(url) == 0 {
		klog.InfoS("initializing splunk with empty url")
		return nil
	}

	if !transport.ValidEndpoint(url) {
		klog.InfoS("initializing splunk with an invalid url",
			"setting", "url")
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

	klog.InfoS("initializing splunk",
		"url", transport.LogURL(url),
		"source", source)

	return &Splunk{
		sender:      transport.NewSender(dependencies),
		url:         url,
		token:       token,
		source:      source,
		sourcetype:  sourcetype,
		index:       index,
		host:        host,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (s *Splunk) Name() string {
	return "Splunk"
}

// SendIncident indexes one structured event per incident message. The
// incident key and state let searches follow an incident from firing to
// resolved.
func (s *Splunk) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	state := "firing"
	if m.Resolved() {
		state = "resolved"
	}
	event := map[string]interface{}{
		"source":      "kwatch",
		"cluster":     s.clusterName,
		"incidentKey": m.AlertKey(s.clusterName),
		"revision":    m.Revision,
		"state":       state,
		"status":      m.Status.String(),
		"severity":    m.Route.Severity,
		"title":       m.ShortText(),
		"message":     m.NoteText(),
		"namespaces":  m.Route.Namespaces,
		"reasons":     m.Route.Reasons,
	}
	if len(m.Output) > 0 {
		event["output"] = m.Output
	}
	addFlags(event, structured.FlagsOf(m))
	return s.send(ctx, event)
}

// SendMessage sends text message to the provider
func (s *Splunk) SendMessage(ctx context.Context, msg string) error {
	return s.send(ctx, map[string]interface{}{
		"message": msg,
		"source":  "kwatch",
	})
}

// send posts one event to the HTTP Event Collector.
func (s *Splunk) send(
	ctx context.Context, event map[string]interface{},
) error {
	payload := splunkPayload{
		Event:      event,
		Source:     s.source,
		Sourcetype: s.sourcetype,
		Index:      s.index,
		Host:       s.host,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = s.sender.Send(ctx, transport.Request{
		Provider: s.Name(), URL: s.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": "Splunk " + s.token,
		},
	})
	return err
}

// addFlags adds the delivery hints a search or alert needs, only when set,
// so an ordinary event keeps its shape.
func addFlags(event map[string]interface{}, f structured.Flags) {
	if f.Opens {
		event["opens"] = true
	}
	if f.PagingOnly {
		event["pagingOnly"] = true
	}
	if f.SkipPaging {
		event["skipPaging"] = true
	}
	if f.Carrier != "" {
		event["carrier"] = f.Carrier
	}
	if f.ReopenWithinSeconds > 0 {
		event["reopenWithinSeconds"] = f.ReopenWithinSeconds
	}
}

// ReceivesPagingOnly says this receiver tracks incidents by key and must
// get a PagingOnly close for an incident it opened, even though it also
// takes plain messages. Delivery reads it next to SkipsPlainMessages.
func (s *Splunk) ReceivesPagingOnly() bool { return true }
