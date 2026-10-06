package sensugo

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
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
	sender    transport.Sender
	url       string
	apiKey    string
	namespace string
	entity    string

	clusterName string
	now         func() time.Time
}

// NewSensugo returns a new Sensugo object

func NewSensugo(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Sensugo {
	url, ok := config["url"].(string)
	if !ok || len(url) == 0 {
		klog.InfoS("initializing sensugo with empty url")
		return nil
	}

	if !transport.ValidEndpoint(url) {
		klog.InfoS("initializing sensugo with an invalid url",
			"setting", "url")
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

	klog.InfoS("initializing sensugo",
		"url", transport.LogURL(url),
		"namespace", namespace)

	return &Sensugo{
		sender: transport.NewSender(dependencies),
		url: strings.TrimRight(url, "/") +
			strings.Replace(sensuAPIPath, "%s", namespace, 1),
		apiKey:      apiKey,
		namespace:   namespace,
		entity:      entity,
		clusterName: clusterName,
		now:         dependencies.Now,
	}
}

// Name returns name of the provider
func (s *Sensugo) Name() string {
	return "Sensu Go"
}

// SendIncident reports one Sensu check per kwatch incident: status 2
// (critical) or 1 (warning) while it is firing and 0 once it resolves, so
// the event clears.
func (s *Sensugo) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	// A plain notice (startup, upgrade, test) or the startup summary is
	// not an incident, and nothing would ever resolve what it opens.
	if m.IsInformational() {
		klog.V(4).InfoS("skipping informational message",
			"component", "delivery", "provider", s.Name())
		return nil
	}
	output := safetext.PlainWithOutput(
		m.NoteText(), safetext.LastOutput(m.Output), "\n\n", safetext.DetailsLimit)
	payload := sensuPayload{
		Entity: sensuEntity{
			Metadata: sensuMetadata{Name: s.entity},
		},
		Check: sensuCheck{
			Metadata: sensuMetadata{Name: m.AlertKey(s.clusterName)},
			Status:   checkStatus(m),
			Output:   output,
			Issued:   s.now().Unix(),
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.sender.Send(ctx, transport.Request{
		Provider: s.Name(), URL: s.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": "Key " + s.apiKey,
		},
	})
	return err
}

// checkStatus maps an incident onto Sensu's exit-code statuses. A plain
// notice is OK (0), so it is recorded without leaving a failing check open.
func checkStatus(m notification.Message) int {
	switch {
	case m.Resolved(), m.IsNotice():
		return 0
	case m.Route.Severity == "critical":
		return 2
	case m.Route.Severity == "" && m.Status == notification.StatusCritical:
		return 2
	}
	return 1
}

// SendMessage sends a plain notice as a passing (OK) check.
func (s *Sensugo) SendMessage(ctx context.Context, msg string) error {
	return s.SendIncident(ctx, notification.Notice(msg))
}

// SkipsPlainMessages implements api.PlainMessageSkipper: plain messages
// become notices, which SendIncident skips.
func (s *Sensugo) SkipsPlainMessages() bool { return true }
