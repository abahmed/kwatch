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
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/ratelimit"
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
		if !transport.ValidEndpoint(s) {
			klog.InfoS("initializing github with an invalid url",
				"setting", "url")
			return nil
		}
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

// titleLimit is GitHub's maximum issue title length.
const titleLimit = 256

// SendIncident opens one issue per incident, comments on updates and
// closes it on recovery.
func (g *Github) SendIncident(
	ctx context.Context, msg notification.Message,
) error {
	return g.issues.Deliver(ctx, g, msg,
		issues.Title(msg, titleLimit), issues.Body(msg))
}

// SendMessage treats a plain message as a notice, which never opens an
// issue.
func (g *Github) SendMessage(ctx context.Context, msg string) error {
	return g.SendIncident(ctx, notification.Notice(msg))
}

// SkipsPlainMessages implements api.PlainMessageSkipper: plain messages
// become notices, which SendIncident skips.
func (g *Github) SkipsPlainMessages() bool { return true }

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

// Reopen implements issues.Reopener: an incident that fails again inside
// its reopen window reopens the issue it closed.
func (g *Github) Reopen(ctx context.Context, id string) error {
	_, err := g.call(ctx, "PATCH",
		g.url+"/"+id, map[string]string{"state": "open"})
	return err
}

func (g *Github) call(
	ctx context.Context, method, url string, payload interface{},
) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	response, err := g.sender.Send(ctx, transport.Request{
		Provider: g.Name(), Method: method, URL: url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": "Bearer " + g.token,
			"Accept":        "application/vnd.github+json",
		},
	})
	if err != nil && rateLimited(err, response) {
		return response, &ratelimit.Error{
			// Not wrapping err: it is marked permanent.
			Provider: g.Name(), StatusCode: 403,
			RetryAfter: waitHint(err, response),
		}
	}
	return response, err
}

// rateLimited reports whether err is GitHub's primary or secondary rate
// limit. GitHub answers those with 403, not 429, so the shared status
// policy calls them permanent. The sender does not hand back response
// headers, so the documented message text is the signal: "API rate limit
// exceeded" (primary) and "secondary rate limit" (secondary).
func rateLimited(err error, body []byte) bool {
	if status, ok := transport.StatusOf(err); !ok ||
		status != 403 {
		return false
	}
	text := strings.ToLower(string(body))
	return strings.Contains(text, "rate limit")
}

// HasThread implements delivery.ThreadLookup.
func (g *Github) HasThread(key string) bool {
	return g.issues.HasThread(key)
}

// SnapshotThreads implements delivery.ThreadStateProvider.
func (g *Github) SnapshotThreads() map[string]string {
	return g.issues.SnapshotThreads()
}

// RestoreThreads implements delivery.ThreadStateProvider.
func (g *Github) RestoreThreads(saved map[string]string) {
	g.issues.RestoreThreads(saved)
}
