package clickup

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/issues"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const clickupAPIURL = "https://api.clickup.com/api/v2"

type clickupPayload struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Priority    *int   `json:"priority,omitempty"`
}

type Clickup struct {
	issues   *issues.Map
	sender   transport.Sender
	api      string
	url      string
	token    string
	listID   string
	priority int

	clusterName string
}

// NewClickup returns a new Clickup object

func NewClickup(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Clickup {
	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing clickup with empty token")
		return nil
	}

	listID, ok := config["listId"].(string)
	if !ok || len(listID) == 0 {
		klog.InfoS("initializing clickup with empty listId")
		return nil
	}

	priority := 0
	switch v := config["priority"].(type) {
	case float64:
		priority = int(v)
	case int:
		priority = v
	case int64:
		priority = int(v)
	}

	klog.InfoS("initializing clickup", "listId", listID, "priority", priority)

	return &Clickup{
		issues:      issues.NewMap(),
		sender:      transport.NewSender(dependencies),
		api:         clickupAPIURL,
		url:         fmt.Sprintf("%s/list/%s/task", clickupAPIURL, listID),
		token:       token,
		listID:      listID,
		priority:    priority,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (g *Clickup) Name() string {
	return "Clickup"
}

// titleLimit is a conservative bound for ClickUp task names.
const titleLimit = 255

// SendIncident opens one task per incident and comments on updates and
// on recovery.
func (g *Clickup) SendIncident(
	ctx context.Context, msg notification.Message,
) error {
	return g.issues.Deliver(ctx, g, msg,
		issues.Title(msg, titleLimit), issues.Body(msg))
}

// SendMessage treats a plain message as a notice, which never opens an
// issue.
func (g *Clickup) SendMessage(ctx context.Context, msg string) error {
	return g.SendIncident(ctx, notification.Notice(msg))
}

// SkipsPlainMessages implements api.PlainMessageSkipper: plain messages
// become notices, which SendIncident skips.
func (g *Clickup) SkipsPlainMessages() bool { return true }

// Create implements issues.Tracker.
func (g *Clickup) Create(
	ctx context.Context, title, body string,
) (string, error) {
	payload := clickupPayload{Name: title, Description: body}
	if g.priority > 0 {
		payload.Priority = &g.priority
	}
	response, err := g.call(ctx, "POST", g.url, payload)
	if err != nil {
		return "", err
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response, &created); err != nil {
		return "", nil
	}
	return created.ID, nil
}

// Comment implements issues.Tracker.
func (g *Clickup) Comment(ctx context.Context, id, body string) error {
	_, err := g.call(ctx, "POST",
		g.api+"/task/"+id+"/comment",
		map[string]string{"comment_text": body})
	return err
}

// Close comments the recovery. ClickUp status names are defined per list, so
// kwatch cannot pick a closed status generically.
func (g *Clickup) Close(ctx context.Context, id, body string) error {
	return g.Comment(ctx, id, body)
}

func (g *Clickup) call(
	ctx context.Context, method, url string, payload interface{},
) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return g.sender.Send(ctx, transport.Request{
		Provider: g.Name(), Method: method, URL: url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": g.token,
		},
	})
}

// SnapshotThreads implements delivery.ThreadStateProvider.
func (g *Clickup) SnapshotThreads() map[string]string {
	return g.issues.SnapshotThreads()
}

// RestoreThreads implements delivery.ThreadStateProvider.
func (g *Clickup) RestoreThreads(saved map[string]string) {
	g.issues.RestoreThreads(saved)
}
