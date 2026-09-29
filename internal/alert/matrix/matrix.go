package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
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
	clockSource clock.Clock
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
		clockSource:    clock.Require(dependencies.Clock),
	}
}

func (m *Matrix) Name() string {
	return "Matrix"
}

func (m *Matrix) SendMessage(ctx context.Context, msg string) error {
	// Rendered messages are plain text built from event data; escape them
	// before they become the HTML body so logs cannot inject markup.
	formatted := strings.ReplaceAll(html.EscapeString(msg), "\n", "<br/>")
	return m.sendBodies(ctx, msg, formatted)
}

func (m *Matrix) SendEvent(ctx context.Context, e *event.Event) error {
	formatted := e.FormatHtml(m.clusterName, m.text)
	return m.sendBodies(ctx, stripHtmlRegex(formatted), formatted)
}

func (m *Matrix) sendBodies(
	ctx context.Context,
	plainMsg, formattedMsg string,
) error {

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
