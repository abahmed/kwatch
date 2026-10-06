package clickup

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/issues"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const clickupAPIURL = "https://api.clickup.com/api/v2"

type clickupPayload struct {
	Name string `json:"name"`
	// MarkdownDescription is rendered as Markdown; the plain
	// "description" field would show the code fence literally.
	MarkdownDescription string `json:"markdown_description"`
	Priority            *int   `json:"priority,omitempty"`
}

type Clickup struct {
	issues   *issues.Map
	sender   transport.Sender
	api      string
	url      string
	token    string
	listID   string
	priority int
	// closeStatus is the list status a resolve moves the task to; empty
	// means the resolve only comments.
	closeStatus string
	// reopenStatus is the list status a recurrence moves a closed task
	// back to; empty means a recurrence opens a new task instead.
	reopenStatus string

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

	// Status names are defined per list, so there is no safe default:
	// without closeStatus a resolve only comments.
	closeStatus, _ := config["closeStatus"].(string)
	closeStatus = strings.TrimSpace(closeStatus)

	reopenStatus, _ := config["reopenStatus"].(string)
	reopenStatus = strings.TrimSpace(reopenStatus)

	klog.InfoS("initializing clickup", "listId", listID,
		"priority", priority, "closeStatus", closeStatus,
		"reopenStatus", reopenStatus)

	return &Clickup{
		issues:       issues.NewMap(),
		sender:       transport.NewSender(dependencies),
		api:          clickupAPIURL,
		url:          fmt.Sprintf("%s/list/%s/task", clickupAPIURL, listID),
		token:        token,
		listID:       listID,
		priority:     priority,
		closeStatus:  closeStatus,
		reopenStatus: reopenStatus,
		clusterName:  clusterName,
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
	payload := clickupPayload{Name: title, MarkdownDescription: body}
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

// Close comments the recovery, then moves the task to the configured
// closeStatus when there is one. A status the list does not have is a
// permanent 400: it is logged and the task is left as it is, since a
// retry cannot fix a wrong name.
func (g *Clickup) Close(ctx context.Context, id, body string) error {
	if err := g.Comment(ctx, id, body); err != nil {
		return err
	}
	if g.closeStatus == "" {
		return nil
	}
	return g.setStatus(ctx, id, g.closeStatus)
}

// ClosesIssues is checked by the issue map: without a closeStatus a
// resolve only comments, so the task stays open.
func (g *Clickup) ClosesIssues() bool { return g.closeStatus != "" }

// CanReopen is checked by the issue map: a task can be reopened only
// when both a closeStatus and a reopenStatus are set.
func (g *Clickup) CanReopen() bool {
	return g.closeStatus != "" && g.reopenStatus != ""
}

// Reopen implements issues.Reopener: it moves the task back to the
// configured reopenStatus before the comment.
func (g *Clickup) Reopen(ctx context.Context, id string) error {
	if g.reopenStatus == "" {
		return nil
	}
	return g.setStatus(ctx, id, g.reopenStatus)
}

// setStatus moves the task to a list status. A status the list does not
// have is logged and ignored, since a retry cannot fix a wrong name.
func (g *Clickup) setStatus(ctx context.Context, id, status string) error {
	_, err := g.call(ctx, "PUT", g.api+"/task/"+id,
		map[string]string{"status": status})
	if err != nil && transport.IsPermanent(err) &&
		!transport.IsNotFound(err) {
		klog.InfoS("clickup could not set the status; left as is",
			"component", "delivery", "provider", g.Name(), "task", id,
			"status", status, "error", err)
		return nil
	}
	return err
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

// HasThread implements delivery.ThreadLookup.
func (g *Clickup) HasThread(key string) bool {
	return g.issues.HasThread(key)
}

// SnapshotThreads implements delivery.ThreadStateProvider.
func (g *Clickup) SnapshotThreads() map[string]string {
	return g.issues.SnapshotThreads()
}

// RestoreThreads implements delivery.ThreadStateProvider.
func (g *Clickup) RestoreThreads(saved map[string]string) {
	g.issues.RestoreThreads(saved)
}
