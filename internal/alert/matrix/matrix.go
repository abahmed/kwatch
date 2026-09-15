package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

var htmlTagRegex = regexp.MustCompile(`<.*?>`)

type Matrix struct {
	sender         transport.Sender
	homeServer     string
	accessToken    string
	internalRoomID string
	title          string
	text           string

	// reference for general app configuration
	clusterName string
}

// NewMatrix returns new Matrix instance

func NewMatrix(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Matrix {
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
		sender:         transport.NewSender(dependencies),
		homeServer:     homeServer,
		accessToken:    accessToken,
		internalRoomID: internalRoomID,
		title:          title,
		text:           text,
		clusterName:    clusterName,
	}
}

func (m *Matrix) Name() string {
	return "Matrix"
}

func (m *Matrix) SendMessage(ctx context.Context, msg string) error {
	return m.sendAPI(ctx, msg)
}

// SendIncident implements delivery.ThreadProvider.
// It renders the incident using the Report model and PlaintextRenderer,
// producing a context-adaptive text message.
func (m *Matrix) SendIncident(
	ctx context.Context,
	inc *model.Incident,
	action model.IncidentAction,
) error {
	return m.SendIncidentWithInsight(ctx, inc, action, nil)
}

// SendIncidentWithInsight implements delivery.InsightThreadProvider, so the
// diagnosis — likely cause, impact, recent changes — is rendered rather than
// dropped on the way to this provider.
func (m *Matrix) SendIncidentWithInsight(
	ctx context.Context,
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	text := message.RenderIncidentWithInsight(
		inc,
		action,
		ins,
		message.NewPlainTextRenderer(),
		m.clusterName,
	)
	if text == "" {
		return nil
	}
	return m.SendMessage(ctx, text)
}

func (m *Matrix) SendEvent(ctx context.Context, e *event.Event) error {
	return m.sendAPI(ctx, e.FormatHtml(m.clusterName, m.text))
}

func (m *Matrix) sendAPI(
	ctx context.Context,
	formattedMsg string,
) error {
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

	_, err = m.sender.Send(ctx, transport.Request{
		Provider: "Matrix",
		Method:   "PUT",
		URL: fmt.Sprintf(
			"%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
			m.homeServer,
			url.PathEscape(m.internalRoomID),
			randomRoomIDPart(24),
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
