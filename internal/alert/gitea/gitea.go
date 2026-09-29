package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/issues"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const giteaAPIURL = "https://gitea.com/api/v1"

type Gitea struct {
	issues *issues.Map
	sender transport.Sender
	url    string
	token  string
	owner  string
	repo   string

	clusterName string
}

// NewGitea returns a new Gitea object

func NewGitea(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Gitea {
	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing gitea with empty token")
		return nil
	}

	owner, ok := config["owner"].(string)
	if !ok || len(owner) == 0 {
		klog.InfoS("initializing gitea with empty owner")
		return nil
	}

	repo, ok := config["repo"].(string)
	if !ok || len(repo) == 0 {
		klog.InfoS("initializing gitea with empty repo")
		return nil
	}

	server := giteaAPIURL
	if s, ok := config["url"].(string); ok && len(s) > 0 {
		server = s
	}

	klog.InfoS("initializing gitea", "url", server, "owner", owner, "repo", repo)

	return &Gitea{
		issues: issues.NewMap(),
		sender: transport.NewSender(dependencies),
		url: fmt.Sprintf(
			"%s/repos/%s/%s/issues",
			strings.TrimRight(server, "/"),
			owner,
			repo,
		),
		token:       token,
		owner:       owner,
		repo:        repo,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (g *Gitea) Name() string {
	return "Gitea"
}

// SendEvent sends event to the provider
// UsesEventDelivery routes problems through SendEvent, which carries the
// action and a stable key so one issue follows one problem.
func (g *Gitea) UsesEventDelivery() {}

// SendEvent opens one issue per problem, comments on updates and closes it
// on recovery.
func (g *Gitea) SendEvent(ctx context.Context, e *event.Event) error {
	return g.issues.Deliver(ctx, g, e, g.issueTitle(e), g.issueBody(e))
}

// SendMessage files a standalone issue for a plain message.
func (g *Gitea) SendMessage(ctx context.Context, msg string) error {
	_, err := g.Create(ctx, g.issueTitle(nil), msg)
	return err
}

func (g *Gitea) issueTitle(e *event.Event) string {
	title := "kwatch alert"
	if e != nil {
		title = e.AlertTitle(200)
	}
	if g.clusterName != "" {
		title = "[" + g.clusterName + "] " + title
	}
	return title
}

func (g *Gitea) issueBody(e *event.Event) string {
	if strings.TrimSpace(e.Narrative) == "" {
		return e.FormatMarkdown(g.clusterName, "", "\n\n")
	}
	return e.AlertBody(g.clusterName)
}

// Create implements issues.Tracker.
func (g *Gitea) Create(
	ctx context.Context, title, body string,
) (string, error) {
	response, err := g.call(ctx, "POST", g.url, map[string]string{
		"title": title, "body": body,
	})
	if err != nil {
		return "", err
	}
	var created struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(response, &created); err != nil ||
		created.Number == 0 {
		return "", nil
	}
	return strconv.Itoa(created.Number), nil
}

// Comment implements issues.Tracker.
func (g *Gitea) Comment(ctx context.Context, id, body string) error {
	_, err := g.call(ctx, "POST",
		g.url+"/"+id+"/comments", map[string]string{"body": body})
	return err
}

// Close implements issues.Tracker.
func (g *Gitea) Close(ctx context.Context, id, body string) error {
	if err := g.Comment(ctx, id, body); err != nil {
		return err
	}
	_, err := g.call(ctx, "PATCH",
		g.url+"/"+id, map[string]string{"state": "closed"})
	return err
}

func (g *Gitea) call(
	ctx context.Context, method, url string, payload interface{},
) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return g.sender.Send(ctx, transport.Request{
		Provider: g.Name(), Method: method, URL: url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": "token " + g.token,
		},
	})
}

// SnapshotThreads implements delivery.ThreadStateProvider.
func (g *Gitea) SnapshotThreads() map[string]string {
	return g.issues.SnapshotThreads()
}

// RestoreThreads implements delivery.ThreadStateProvider.
func (g *Gitea) RestoreThreads(saved map[string]string) {
	g.issues.RestoreThreads(saved)
}
