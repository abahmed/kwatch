package signl4

import (
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const signl4APIURL = "https://connect.signl4.com/webhook"

type signl4Payload struct {
	Title     string `json:"title"`
	Message   string `json:"message"`
	Severity  string `json:"severity,omitempty"`
	User      string `json:"user,omitempty"`
	XS4Status string `json:"X-S4-Status,omitempty"`
}

type Signl4 struct {
	url        string
	teamSecret string
	title      string
	user       string

	appCfg *config.App
}

// NewSignl4 returns a new Signl4 object
func NewSignl4(config map[string]interface{}, appCfg *config.App) *Signl4 {
	teamSecret, ok := config["teamSecret"].(string)
	if !ok || len(teamSecret) == 0 {
		klog.InfoS("initializing signl4 with empty teamSecret")
		return nil
	}

	server := signl4APIURL
	if s, ok := config["url"].(string); ok && len(s) > 0 {
		server = s
	}

	title, _ := config["title"].(string)
	user, _ := config["user"].(string)

	klog.InfoS("initializing signl4", "title", title)

	return &Signl4{
		url:        strings.TrimRight(server, "/") + "/" + teamSecret,
		teamSecret: teamSecret,
		title:      title,
		user:       user,
		appCfg:     appCfg,
	}
}

// Name returns name of the provider
func (s *Signl4) Name() string {
	return "SIGNL4"
}

// SendEvent sends event to the provider
func (s *Signl4) SendEvent(e *event.Event) error {
	msg := e.FormatText(s.appCfg.ClusterName, "")
	return s.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (s *Signl4) SendMessage(msg string) error {
	title := s.title
	if len(title) == 0 {
		title = "kwatch alert"
	}

	payload := signl4Payload{
		Title:     title,
		Message:   msg,
		Severity:  "critical",
		User:      s.user,
		XS4Status: "new",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(s.Name(), s.url, body, "application/json", nil)
	return err
}
