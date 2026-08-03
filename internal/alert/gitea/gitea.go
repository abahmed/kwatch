package gitea

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const giteaAPIURL = "https://gitea.com/api/v1"

type giteaPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type Gitea struct {
	url   string
	token string
	owner string
	repo  string

	appCfg *config.App
}

// NewGitea returns a new Gitea object
func NewGitea(config map[string]interface{}, appCfg *config.App) *Gitea {
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
		url:    fmt.Sprintf("%s/repos/%s/%s/issues", strings.TrimRight(server, "/"), owner, repo),
		token:  token,
		owner:  owner,
		repo:   repo,
		appCfg: appCfg,
	}
}

// Name returns name of the provider
func (g *Gitea) Name() string {
	return "Gitea"
}

// SendEvent sends event to the provider
func (g *Gitea) SendEvent(e *event.Event) error {
	msg := e.FormatMarkdown(g.appCfg.ClusterName, "", "\n\n")
	return g.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (g *Gitea) SendMessage(msg string) error {
	title := fmt.Sprintf("kwatch alert: %s", g.appCfg.ClusterName)
	if g.appCfg.ClusterName == "" {
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

	_, err = util.Post(g.Name(), g.url, body, "application/json", map[string]string{
		"Authorization": "token " + g.token,
	})
	return err
}
