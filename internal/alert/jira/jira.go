package jira

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

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
func (s *Jira) Name() string {
	return "Jira"
}

// SendEvent sends event to the provider
func (s *Jira) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(s.clusterName, "")
	return s.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (s *Jira) SendMessage(ctx context.Context, msg string) error {
	title := "kwatch alert"
	if len(s.clusterName) > 0 {
		title = "kwatch alert: " + s.clusterName
	}

	payload := jiraPayload{
		Fields: jiraFields{
			Project:     map[string]string{"key": s.projectKey},
			IssueType:   map[string]string{"name": s.issueType},
			Summary:     title,
			Description: msg,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(s.user+":"+s.apiToken))
	_, err = s.sender.Send(ctx, transport.Request{
		Provider: s.Name(), URL: s.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": auth,
		},
	})
	return err
}
