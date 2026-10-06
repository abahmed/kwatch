package wecom

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

// wecomTextLimit is WeCom's markdown content limit in bytes.
const wecomTextLimit = 4096

type wecomPayload struct {
	MsgType  string            `json:"msgtype"`
	Markdown map[string]string `json:"markdown"`
}

// rateLimitCodes are the documented frequency-limit codes; every other
// body error is a permanent rejection.
var rateLimitCodes = map[int]bool{45009: true, 45033: true}

type wecomResponse struct {
	ErrorCode int    `json:"errcode"`
	ErrorText string `json:"errmsg"`
}

type Wecom struct {
	sender  transport.Sender
	webhook string

	clusterName string
}

// NewWecom returns a new Wecom object

func NewWecom(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Wecom {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing wecom with empty webhook")
		return nil
	}

	if !transport.ValidEndpoint(webhook) {
		klog.InfoS("initializing wecom with an invalid webhook",
			"setting", "webhook")
		return nil
	}

	klog.InfoS("initializing wecom with webhook configured")

	return &Wecom{
		sender:      transport.NewSender(dependencies),
		webhook:     webhook,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (s *Wecom) Name() string {
	return "WeCom"
}

// SendIncident sends the incident narrative as markdown, with the
// application output in a code block, within WeCom's content limit.
func (s *Wecom) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	return s.SendMessage(ctx, safetext.NoteWithOutput(
		safetext.WeCom(m.NoteText()),
		safetext.Lines(m.Output, safetext.WeCom),
		"\n\n", wecomTextLimit))
}

// SendMessage sends text message to the provider
func (s *Wecom) SendMessage(ctx context.Context, msg string) error {
	payload := wecomPayload{
		MsgType: "markdown",
		Markdown: map[string]string{
			"content": msg,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	responseBody, err := s.sender.Send(ctx, transport.Request{
		Provider: s.Name(), URL: s.webhook, Body: body,
		ContentType: "application/json",
	})
	if err != nil {
		return err
	}
	if len(responseBody) == 0 {
		return nil
	}
	var response wecomResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return fmt.Errorf("wecom returned invalid response")
	}
	if response.ErrorCode != 0 {
		err := fmt.Errorf(
			"wecom request failed with code %d", response.ErrorCode,
		)
		if rateLimitCodes[response.ErrorCode] {
			return &ratelimit.Error{Provider: "WeCom",
				StatusCode: ratelimit.InBodyStatus, Err: err}
		}
		return transport.Permanent(err)
	}
	return nil
}
