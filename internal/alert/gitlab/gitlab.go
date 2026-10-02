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
	"github.com/abahmed/kwatch/internal/notification"
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
		if !transport.ValidEndpoint(s) {
			klog.InfoS("initializing gitlab with an invalid url",
				"setting", "url")
			return nil
		}
		server = s
	}

	klog.InfoS("initializing gitlab",
		"url", transport.LogURL(server),
		"projectId", projectID)

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

// titleLimit is GitLab's maximum issue title length.
const titleLimit = 255

// SendIncident opens one issue per incident, comments on updates and
// closes it on recovery.
func (g *Gitlab) SendIncident(
	ctx context.Context, msg notification.Message,
) error {
	return g.issues.Deliver(ctx, g, msg,
		issues.Title(msg, titleLimit), issues.Body(msg))
}

// SendMessage treats a plain message as a notice, which never opens an
// issue.
func (g *Gitlab) SendMessage(ctx context.Context, msg string) error {
	return g.SendIncident(ctx, notification.Notice(msg))
}

// SkipsPlainMessages implements api.PlainMessageSkipper: plain messages
// become notices, which SendIncident skips.
func (g *Gitlab) SkipsPlainMessages() bool { return true }

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
