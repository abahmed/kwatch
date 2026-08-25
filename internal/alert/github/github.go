package github

import (
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const githubAPIURL = "https://api.github.com"

type githubPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type Github struct {
	url   string
	token string
	owner string
	repo  string

	appCfg *config.App
}

// NewGithub returns a new Github object
func NewGithub(config map[string]interface{}, appCfg *config.App) *Github {
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
		url:    fmt.Sprintf("%s/repos/%s/%s/issues", server, owner, repo),
		token:  token,
		owner:  owner,
		repo:   repo,
		appCfg: appCfg,
	}
}

// Name returns name of the provider
func (g *Github) Name() string {
	return "Github"
}

// SendEvent sends event to the provider
func (g *Github) SendEvent(e *event.Event) error {
	msg := e.FormatMarkdown(g.appCfg.ClusterName, "", "\n\n")
	return g.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (g *Github) SendMessage(msg string) error {
	title := fmt.Sprintf("kwatch alert: %s", g.appCfg.ClusterName)
	if g.appCfg.ClusterName == "" {
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

	_, err = util.Post(g.Name(), g.url, body, "application/json", map[string]string{
		"Authorization": "Bearer " + g.token,
		"Accept":        "application/vnd.github+json",
	})
	return err
}
