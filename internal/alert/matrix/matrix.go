package matrix

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

var htmlTagRegex = regexp.MustCompile(`<.*?>`)

type Matrix struct {
	homeServer     string
	accessToken    string
	internalRoomID string
	title          string
	text           string

	// reference for general app configuration
	appCfg *config.App
}

// NewMatrix returns new Matrix instance
func NewMatrix(config map[string]interface{}, appCfg *config.App) *Matrix {
	homeServer, ok := config["homeServer"].(string)
	if !ok || len(homeServer) == 0 {
		klog.InfoS("initializing matrix with empty homeServer")
		return nil
	}

	accessToken, ok := config["accessToken"].(string)
	if !ok || len(accessToken) == 0 {
		klog.InfoS("initializing matrix with empty accessToken")
		return nil
	}

	internalRoomID, ok := config["internalRoomId"].(string)
	if !ok || len(internalRoomID) == 0 {
		klog.InfoS("initializing matrix with empty internalRoomId")
		return nil
	}

	title, _ := config["title"].(string)
	text, _ := config["text"].(string)

	return &Matrix{
		homeServer:     homeServer,
		accessToken:    accessToken,
		internalRoomID: internalRoomID,
		title:          title,
		text:           text,
		appCfg:         appCfg,
	}
}

func (m *Matrix) Name() string {
	return "Matrix"
}

func (m *Matrix) SendMessage(msg string) error {
	return m.sendAPI(msg)
}

// SendIncident implements alert.ThreadProvider.
// It renders the incident using the Report model and PlaintextRenderer,
// producing a context-adaptive text message.
func (m *Matrix) SendIncident(
	inc *model.Incident,
	action model.IncidentAction,
) error {
	return m.SendIncidentWithInsight(inc, action, nil)
}

// SendIncidentWithInsight implements alert.InsightThreadProvider, so the
// diagnosis — likely cause, impact, recent changes — is rendered rather than
// dropped on the way to this provider.
func (m *Matrix) SendIncidentWithInsight(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	text := util.RenderIncidentWithInsight(
		inc,
		action,
		ins,
		message.NewPlainTextRenderer(),
		m.appCfg.ClusterName,
	)
	if text == "" {
		return nil
	}
	return m.SendMessage(text)
}

func (m *Matrix) SendEvent(e *event.Event) error {
	return m.sendAPI(e.FormatHtml(m.appCfg.ClusterName, m.text))
}

func (m *Matrix) sendAPI(formattedMsg string) error {
	plainMsg := stripHtmlRegex(formattedMsg)

	payload := struct {
		Msgtype       string `json:"msgtype"`
		Format        string `json:"format"`
		Body          string `json:"body"`
		FormattedBody string `json:"formatted_body"`
	}{
		Msgtype:       "m.text",
		Format:        "org.matrix.custom.html",
		Body:          plainMsg,
		FormattedBody: formattedMsg,
	}

	msgBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Send(util.Request{
		Provider: "Matrix",
		Method:   http.MethodPut,
		URL: fmt.Sprintf(
			"%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
			m.homeServer,
			url.PathEscape(m.internalRoomID),
			k8s.RandomString(24),
		),
		Body:    msgBytes,
		Headers: map[string]string{"Authorization": "Bearer " + m.accessToken},
	})
	return err
}

// This method uses a regular expression to remove HTML tags.
func stripHtmlRegex(s string) string {
	return htmlTagRegex.ReplaceAllString(s, "")
}
