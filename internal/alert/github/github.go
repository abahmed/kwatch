package github

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

const githubAPIURL = "https://api.github.com"

type Github struct {
	issues *issues.Map
	sender transport.Sender
	url    string
	token  string
	owner  string
	repo   string

	clusterName string
}

// NewGithub returns a new Github object

func NewGithub(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Github {
	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing github with empty token")
		return nil
	}

	owner, ok := config["owner"].(string)
	if !ok || len(owner) == 0 {
		klog.InfoS("initializing github with empty owner")
		return nil
	}

	repo, ok := config["repo"].(string)
	if !ok || len(repo) == 0 {
		klog.InfoS("initializing github with empty repo")
		return nil
	}

	server := githubAPIURL
	if s, ok := config["url"].(string); ok && len(s) > 0 {
		server = s
	}

	klog.InfoS("initializing github", "owner", owner, "repo", repo)

	return &Github{
		issues:      issues.NewMap(),
		sender:      transport.NewSender(dependencies),
		url:         fmt.Sprintf("%s/repos/%s/%s/issues", server, owner, repo),
		token:       token,
		owner:       owner,
		repo:        repo,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (g *Github) Name() string {
	return "Github"
}

// SendEvent sends event to the provider
// UsesEventDelivery routes incidents through SendEvent, which carries the
// action and a stable key so one issue follows one incident.
func (g *Github) UsesEventDelivery() {}

// SendEvent opens one issue per incident, comments on updates and closes it
// on recovery.
func (g *Github) SendEvent(ctx context.Context, e *event.Event) error {
	return g.issues.Deliver(ctx, g, e, g.issueTitle(e), g.issueBody(e))
}

// SendMessage files a standalone issue for a plain message.
func (g *Github) SendMessage(ctx context.Context, msg string) error {
	_, err := g.Create(ctx, g.issueTitle(nil), msg)
	return err
}

func (g *Github) issueTitle(e *event.Event) string {
	title := "kwatch alert"
	if e != nil {
		title = e.AlertTitle(200)
	}
	if g.clusterName != "" {
		title = "[" + g.clusterName + "] " + title
	}
	return title
}

func (g *Github) issueBody(e *event.Event) string {
	if strings.TrimSpace(e.Narrative) == "" {
		return e.FormatMarkdown(g.clusterName, "", "\n\n")
	}
	return e.AlertBody(g.clusterName)
}

// Create implements issues.Tracker.
func (g *Github) Create(
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
func (g *Github) Comment(ctx context.Context, id, body string) error {
	_, err := g.call(ctx, "POST",
		g.url+"/"+id+"/comments", map[string]string{"body": body})
	return err
}

// Close implements issues.Tracker.
func (g *Github) Close(ctx context.Context, id, body string) error {
	if err := g.Comment(ctx, id, body); err != nil {
		return err
	}
	_, err := g.call(ctx, "PATCH",
		g.url+"/"+id, map[string]string{"state": "closed"})
	return err
}

func (g *Github) call(
	ctx context.Context, method, url string, payload interface{},
) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return g.sender.Send(ctx, transport.Request{
		Provider: g.Name(), Method: method, URL: url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": "Bearer " + g.token,
			"Accept":        "application/vnd.github+json",
		},
	})
}

// SnapshotThreads implements delivery.ThreadStateProvider.
func (g *Github) SnapshotThreads() map[string]string {
	return g.issues.SnapshotThreads()
}

// RestoreThreads implements delivery.ThreadStateProvider.
func (g *Github) RestoreThreads(saved map[string]string) {
	g.issues.RestoreThreads(saved)
}
