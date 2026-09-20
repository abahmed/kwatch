package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const gitlabAPIURL = "https://gitlab.com/api/v4"

type gitlabPayload struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type Gitlab struct {
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
		sender: transport.NewSender(dependencies),
		url: fmt.Sprintf(
			"%s/projects/%s/issues",
			strings.TrimRight(server, "/"),
			projectID,
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
func (g *Gitlab) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatMarkdown(g.clusterName, "", "\n\n")
	return g.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (g *Gitlab) SendMessage(ctx context.Context, msg string) error {
	title := fmt.Sprintf("kwatch alert: %s", g.clusterName)
	if g.clusterName == "" {
		title = "kwatch alert"
	}

	payload := gitlabPayload{
		Title:       title,
		Description: msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = g.sender.Send(ctx, transport.Request{
		Provider: g.Name(), URL: g.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"PRIVATE-TOKEN": g.token,
		},
	})
	return err
}
