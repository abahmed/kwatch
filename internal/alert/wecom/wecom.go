package wecom

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

type wecomPayload struct {
	MsgType  string            `json:"msgtype"`
	Markdown map[string]string `json:"markdown"`
}

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

// SendEvent sends event to the provider
func (s *Wecom) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(s.clusterName, "")
	return s.SendMessage(ctx, msg)
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
		return fmt.Errorf("wecom request failed with code %d", response.ErrorCode)
	}
	return nil
}
