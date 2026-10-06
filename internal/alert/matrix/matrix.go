package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

type Matrix struct {
	sender         transport.Sender
	homeServer     string
	accessToken    string
	internalRoomID string

	// reference for general app configuration
	clusterName string
	clockSource clock.Clock

	// Plain messages carry no logical identity, so each send gets a unique
	// one from this instance nonce and counter.
	nonce   string
	counter atomic.Uint64
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

	if !transport.ValidEndpoint(homeServer) {
		klog.InfoS("initializing matrix with an invalid homeServer",
			"setting", "homeServer")
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

	return &Matrix{
		nonce:          newNonce(),
		sender:         transport.NewSender(dependencies),
		homeServer:     strings.TrimRight(homeServer, "/"),
		accessToken:    accessToken,
		internalRoomID: internalRoomID,
		clusterName:    clusterName,
		clockSource:    clock.Require(dependencies.Clock),
	}
}

func (m *Matrix) Name() string {
	return "Matrix"
}

// SendMessage sends a plain operator message. The text is escaped before
// it becomes the HTML body so no markup can be injected.
func (m *Matrix) SendMessage(ctx context.Context, msg string) error {
	msg = safetext.Matrix(msg)
	identity := fmt.Sprintf("plain|%s|%d", m.nonce, m.counter.Add(1))
	return m.sendBodies(ctx, identity, msg, escapeHTML(msg))
}

// SendIncident posts the incident narrative. The plain body is the Note;
// the HTML body is the escaped Note, followed by the escaped application
// output in a kwatch-generated code block.
func (m *Matrix) SendIncident(
	ctx context.Context, msg notification.Message,
) error {
	plain := safetext.Matrix(msg.NoteText())
	formatted := escapeHTML(plain)
	if len(msg.Output) > 0 {
		output := safetext.Matrix(
			strings.Join(msg.Output, "\n"))
		plain += "\n\n" + output
		formatted += "<pre><code>" + html.EscapeString(output) +
			"</code></pre>"
	}
	identity := fmt.Sprintf("%s|%d|%s", msg.AlertKey(m.clusterName), msg.Revision,
		msg.Status)
	return m.sendBodies(ctx, identity, plain, formatted)
}

// escapeHTML escapes event-derived text and keeps line breaks as the only
// generated tag.
func escapeHTML(text string) string {
	return strings.ReplaceAll(html.EscapeString(text), "\n", "<br/>")
}

func (m *Matrix) sendBodies(
	ctx context.Context,
	identity, plainMsg, formattedMsg string,
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

	txnID := transactionID(m.internalRoomID, identity, plainMsg, formattedMsg)
	body, err := m.sender.Send(ctx, transport.Request{
		Provider: "Matrix",
		Method:   "PUT",
		URL: fmt.Sprintf(
			"%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
			m.homeServer,
			url.PathEscape(m.internalRoomID),
			txnID,
		),
		Body:    msgBytes,
		Headers: map[string]string{"Authorization": "Bearer " + m.accessToken},
		// Matrix reports the back-off in the JSON body, in milliseconds.
		RetryAfterFromBody: retryAfterFromBody,
	})
	if err != nil {
		return err
	}
	return checkMatrixBody(body)
}

// checkMatrixBody rejects a 2xx response that still carries a Matrix
// error code, so a proxied failure is not counted as delivered.
func checkMatrixBody(body []byte) error {
	var reply struct {
		ErrCode string `json:"errcode"`
	}
	if len(body) == 0 || json.Unmarshal(body, &reply) != nil ||
		reply.ErrCode == "" {
		return nil
	}
	return transport.Permanent(fmt.Errorf(
		"call to Matrix returned error code %s", reply.ErrCode))
}

// retryAfterFromBody reads retry_after_ms from a 429 answer. Zero means
// the body has none, so the default backoff applies.
func retryAfterFromBody(body []byte) time.Duration {
	var reply struct {
		RetryAfterMs int64 `json:"retry_after_ms"`
	}
	if json.Unmarshal(body, &reply) != nil || reply.RetryAfterMs <= 0 {
		return 0
	}
	const maxWait = time.Hour
	wait := time.Duration(reply.RetryAfterMs) * time.Millisecond
	if wait > maxWait {
		return maxWait
	}
	return wait
}
