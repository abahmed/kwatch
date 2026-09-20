package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const giteaAPIURL = "https://gitea.com/api/v1"

type giteaPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type Gitea struct {
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
func (g *Gitea) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatMarkdown(g.clusterName, "", "\n\n")
	return g.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (g *Gitea) SendMessage(ctx context.Context, msg string) error {
	title := fmt.Sprintf("kwatch alert: %s", g.clusterName)
	if g.clusterName == "" {
		title = "kwatch alert"
	}

	payload := giteaPayload{
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
			"Authorization": "token " + g.token,
		},
	})
	return err
}
