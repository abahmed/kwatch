package clickup

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/issues"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
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

// SendEvent sends event to the provider
// UsesEventDelivery routes incidents through SendEvent, which carries the
// action and a stable key so one issue follows one incident.
func (g *Clickup) UsesEventDelivery() {}

// SendEvent opens one issue per incident, comments on updates and comments
// on recovery.
func (g *Clickup) SendEvent(ctx context.Context, e *event.Event) error {
	return g.issues.Deliver(ctx, g, e, g.issueTitle(e), g.issueBody(e))
}

// SendMessage files a standalone issue for a plain message.
func (g *Clickup) SendMessage(ctx context.Context, msg string) error {
	_, err := g.Create(ctx, g.issueTitle(nil), msg)
	return err
}

func (g *Clickup) issueTitle(e *event.Event) string {
	title := "kwatch alert"
	if e != nil {
		title = e.AlertTitle(200)
	}
	if g.clusterName != "" {
		title = "[" + g.clusterName + "] " + title
	}
	return title
}

func (g *Clickup) issueBody(e *event.Event) string {
	if strings.TrimSpace(e.Narrative) == "" {
		return e.FormatMarkdown(g.clusterName, "", "\n\n")
	}
	return e.AlertBody(g.clusterName)
}

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
		clickupAPIURL+"/task/"+id+"/comment",
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
