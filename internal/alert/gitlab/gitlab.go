package gitlab

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const gitlabAPIURL = "https://gitlab.com/api/v4"

type gitlabPayload struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type Gitlab struct {
	url       string
	token     string
	projectID string

	appCfg *config.App
}

// NewGitlab returns a new Gitlab object
func NewGitlab(config map[string]interface{}, appCfg *config.App) *Gitlab {
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
		url:       fmt.Sprintf("%s/projects/%s/issues", strings.TrimRight(server, "/"), projectID),
		token:     token,
		projectID: projectID,
		appCfg:    appCfg,
	}
}

// Name returns name of the provider
func (g *Gitlab) Name() string {
	return "Gitlab"
}

// SendEvent sends event to the provider
func (g *Gitlab) SendEvent(e *event.Event) error {
	msg := e.FormatMarkdown(g.appCfg.ClusterName, "", "\n\n")
	return g.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (g *Gitlab) SendMessage(msg string) error {
	title := fmt.Sprintf("kwatch alert: %s", g.appCfg.ClusterName)
	if g.appCfg.ClusterName == "" {
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

	_, err = util.Post(g.Name(), g.url, body, "application/json", map[string]string{
		"PRIVATE-TOKEN": g.token,
	})
	return err
}
