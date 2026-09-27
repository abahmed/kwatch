package jira

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/issues"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
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

	klog.InfoS("initializing jira", "url", url, "projectKey", projectKey, "issueType", issueType)

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

// SendEvent sends event to the provider
// UsesEventDelivery routes incidents through SendEvent, which carries the
// action and a stable key so one issue follows one incident.
func (g *Jira) UsesEventDelivery() {}

// SendEvent opens one issue per incident, comments on updates and comments
// on recovery.
func (g *Jira) SendEvent(ctx context.Context, e *event.Event) error {
	return g.issues.Deliver(ctx, g, e, g.issueTitle(e), g.issueBody(e))
}

// SendMessage files a standalone issue for a plain message.
func (g *Jira) SendMessage(ctx context.Context, msg string) error {
	_, err := g.Create(ctx, g.issueTitle(nil), msg)
	return err
}

func (g *Jira) issueTitle(e *event.Event) string {
	title := "kwatch alert"
	if e != nil {
		title = e.AlertTitle(200)
	}
	if g.clusterName != "" {
		title = "[" + g.clusterName + "] " + title
	}
	return title
}

func (g *Jira) issueBody(e *event.Event) string {
	if strings.TrimSpace(e.Narrative) == "" {
		return e.FormatText(g.clusterName, "")
	}
	return e.AlertBody(g.clusterName)
}

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
