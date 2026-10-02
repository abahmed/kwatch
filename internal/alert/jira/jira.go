package jira

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/issues"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const jiraAPIPath = "/rest/api/2/issue"

type jiraFields struct {
	Project     map[string]string `json:"project"`
	IssueType   map[string]string `json:"issuetype"`
	Summary     string            `json:"summary"`
	Description string            `json:"description"`
}

type jiraPayload struct {
	Fields jiraFields `json:"fields"`
}

type Jira struct {
	issues     *issues.Map
	sender     transport.Sender
	url        string
	user       string
	apiToken   string
	projectKey string
	issueType  string

	clusterName string
}

// NewJira returns a new Jira object

func NewJira(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Jira {
	url, ok := config["url"].(string)
	if !ok || len(url) == 0 {
		klog.InfoS("initializing jira with empty url")
		return nil
	}

	if !transport.ValidEndpoint(url) {
		klog.InfoS("initializing jira with an invalid url",
			"setting", "url")
		return nil
	}

	user, ok := config["user"].(string)
	if !ok || len(user) == 0 {
		klog.InfoS("initializing jira with empty user")
		return nil
	}

	apiToken, ok := config["apiToken"].(string)
	if !ok || len(apiToken) == 0 {
		klog.InfoS("initializing jira with empty apiToken")
		return nil
	}

	projectKey, ok := config["projectKey"].(string)
	if !ok || len(projectKey) == 0 {
		klog.InfoS("initializing jira with empty projectKey")
		return nil
	}

	issueType, _ := config["issueType"].(string)
	if len(issueType) == 0 {
		issueType = "Task"
	}

	klog.InfoS("initializing jira",
		"url", transport.LogURL(url),
		"projectKey", projectKey,
		"issueType", issueType)

	return &Jira{
		issues:      issues.NewMap(),
		sender:      transport.NewSender(dependencies),
		url:         strings.TrimRight(url, "/") + jiraAPIPath,
		user:        user,
		apiToken:    apiToken,
		projectKey:  projectKey,
		issueType:   issueType,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (g *Jira) Name() string {
	return "Jira"
}

// titleLimit is Jira's maximum summary length.
const titleLimit = 255

// SendIncident opens one issue per incident and comments on updates and
// on recovery.
func (g *Jira) SendIncident(
	ctx context.Context, msg notification.Message,
) error {
	return g.issues.Deliver(ctx, g, msg,
		issues.Title(msg, titleLimit),
		issues.FencedBody(msg, "{noformat}"))
}

// SendMessage treats a plain message as a notice, which never opens an
// issue.
func (g *Jira) SendMessage(ctx context.Context, msg string) error {
	return g.SendIncident(ctx, notification.Notice(msg))
}

// SkipsPlainMessages implements api.PlainMessageSkipper: plain messages
// become notices, which SendIncident skips.
func (g *Jira) SkipsPlainMessages() bool { return true }

// Create implements issues.Tracker.
func (g *Jira) Create(
	ctx context.Context, title, body string,
) (string, error) {
	response, err := g.call(ctx, "POST", g.url, jiraPayload{
		Fields: jiraFields{
			Project:     map[string]string{"key": g.projectKey},
			IssueType:   map[string]string{"name": g.issueType},
			Summary:     title,
			Description: body,
		},
	})
	if err != nil {
		return "", err
	}
	var created struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(response, &created); err != nil {
		return "", nil
	}
	return created.Key, nil
}

// Comment implements issues.Tracker.
func (g *Jira) Comment(ctx context.Context, id, body string) error {
	_, err := g.call(ctx, "POST",
		g.url+"/"+id+"/comment", map[string]string{"body": body})
	return err
}

// Close comments the recovery. Jira workflows name their transitions per
// project, so kwatch cannot pick a closing transition generically.
func (g *Jira) Close(ctx context.Context, id, body string) error {
	return g.Comment(ctx, id, body)
}

func (g *Jira) call(
	ctx context.Context, method, url string, payload interface{},
) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	auth := "Basic " + base64.StdEncoding.EncodeToString(
		[]byte(g.user+":"+g.apiToken),
	)
	return g.sender.Send(ctx, transport.Request{
		Provider: g.Name(), Method: method, URL: url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": auth,
		},
	})
}

// SnapshotThreads implements delivery.ThreadStateProvider.
func (g *Jira) SnapshotThreads() map[string]string {
	return g.issues.SnapshotThreads()
}

// RestoreThreads implements delivery.ThreadStateProvider.
func (g *Jira) RestoreThreads(saved map[string]string) {
	g.issues.RestoreThreads(saved)
}
