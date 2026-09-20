package github

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const githubAPIURL = "https://api.github.com"

type githubPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type Github struct {
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
func (g *Github) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatMarkdown(g.clusterName, "", "\n\n")
	return g.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (g *Github) SendMessage(ctx context.Context, msg string) error {
	title := fmt.Sprintf("kwatch alert: %s", g.clusterName)
	if g.clusterName == "" {
		title = "kwatch alert"
	}

	payload := githubPayload{
		Title: title,
		Body:  msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = g.sender.Send(ctx, transport.Request{
		Provider: g.Name(), URL: g.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": "Bearer " + g.token,
			"Accept":        "application/vnd.github+json",
		},
	})
	return err
}
