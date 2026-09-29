package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/issues"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const gitlabAPIURL = "https://gitlab.com/api/v4"

type Gitlab struct {
	issues    *issues.Map
	sender    transport.Sender
	url       string
	token     string
	projectID string

	clusterName string
}

// NewGitlab returns a new Gitlab object

func NewGitlab(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Gitlab {
	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing gitlab with empty token")
		return nil
	}

	projectID, ok := config["projectId"].(string)
	if !ok || len(projectID) == 0 {
		klog.InfoS("initializing gitlab with empty projectId")
		return nil
	}

	server := gitlabAPIURL
	if s, ok := config["url"].(string); ok && len(s) > 0 {
		server = s
	}

	klog.InfoS("initializing gitlab", "url", server, "projectId", projectID)

	return &Gitlab{
		issues: issues.NewMap(),
		sender: transport.NewSender(dependencies),
		url: fmt.Sprintf(
			"%s/projects/%s/issues",
			strings.TrimRight(server, "/"),
			// A "group/project" path must be one escaped path segment.
			url.PathEscape(projectID),
		),
		token:       token,
		projectID:   projectID,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (g *Gitlab) Name() string {
	return "Gitlab"
}

// SendEvent sends event to the provider
// UsesEventDelivery routes problems through SendEvent, which carries the
// action and a stable key so one issue follows one problem.
func (g *Gitlab) UsesEventDelivery() {}

// SendEvent opens one issue per problem, comments on updates and closes it
// on recovery.
func (g *Gitlab) SendEvent(ctx context.Context, e *event.Event) error {
	return g.issues.Deliver(ctx, g, e, g.issueTitle(e), g.issueBody(e))
}

// SendMessage files a standalone issue for a plain message.
func (g *Gitlab) SendMessage(ctx context.Context, msg string) error {
	_, err := g.Create(ctx, g.issueTitle(nil), msg)
	return err
}

func (g *Gitlab) issueTitle(e *event.Event) string {
	title := "kwatch alert"
	if e != nil {
		title = e.AlertTitle(200)
	}
	if g.clusterName != "" {
		title = "[" + g.clusterName + "] " + title
	}
	return title
}

func (g *Gitlab) issueBody(e *event.Event) string {
	if strings.TrimSpace(e.Narrative) == "" {
		return e.FormatMarkdown(g.clusterName, "", "\n\n")
	}
	return e.AlertBody(g.clusterName)
}

// Create implements issues.Tracker.
func (g *Gitlab) Create(
	ctx context.Context, title, body string,
) (string, error) {
	response, err := g.call(ctx, "POST", g.url, map[string]string{
		"title": title, "description": body,
	})
	if err != nil {
		return "", err
	}
	var created struct {
		IID int `json:"iid"`
	}
	if err := json.Unmarshal(response, &created); err != nil ||
		created.IID == 0 {
		return "", nil
	}
	return strconv.Itoa(created.IID), nil
}

// Comment implements issues.Tracker.
func (g *Gitlab) Comment(ctx context.Context, id, body string) error {
	_, err := g.call(ctx, "POST",
		g.url+"/"+id+"/notes", map[string]string{"body": body})
	return err
}

// Close implements issues.Tracker.
func (g *Gitlab) Close(ctx context.Context, id, body string) error {
	if err := g.Comment(ctx, id, body); err != nil {
		return err
	}
	_, err := g.call(ctx, "PUT",
		g.url+"/"+id, map[string]string{"state_event": "close"})
	return err
}

func (g *Gitlab) call(
	ctx context.Context, method, url string, payload interface{},
) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return g.sender.Send(ctx, transport.Request{
		Provider: g.Name(), Method: method, URL: url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"PRIVATE-TOKEN": g.token,
		},
	})
}

// SnapshotThreads implements delivery.ThreadStateProvider.
func (g *Gitlab) SnapshotThreads() map[string]string {
	return g.issues.SnapshotThreads()
}

// RestoreThreads implements delivery.ThreadStateProvider.
func (g *Gitlab) RestoreThreads(saved map[string]string) {
	g.issues.RestoreThreads(saved)
}
