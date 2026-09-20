package ifttt

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const iftttAPIURL = "https://maker.ifttt.com/trigger/%s/with/key/%s"

type iftttPayload struct {
	Value1 string `json:"value1"`
	Value2 string `json:"value2"`
	Value3 string `json:"value3"`
}

type iftttResponse struct {
	Errors []json.RawMessage `json:"errors"`
}

type Ifttt struct {
	sender transport.Sender
	url    string
	key    string
	event  string

	clusterName string
}

// NewIfttt returns a new Ifttt object

func NewIfttt(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Ifttt {
	key, ok := config["key"].(string)
	if !ok || len(key) == 0 {
		klog.InfoS("initializing ifttt with empty key")
		return nil
	}

	eventName := "kwatch"
	if e, ok := config["event"].(string); ok && len(e) > 0 {
		eventName = e
	}

	klog.InfoS("initializing ifttt", "event", eventName)

	return &Ifttt{
		sender:      transport.NewSender(dependencies),
		url:         fmt.Sprintf(iftttAPIURL, eventName, key),
		key:         key,
		event:       eventName,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (i *Ifttt) Name() string {
	return "Ifttt"
}

// SendEvent sends event to the provider
func (i *Ifttt) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(i.clusterName, "")
	return i.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (i *Ifttt) SendMessage(ctx context.Context, msg string) error {
	payload := iftttPayload{
		Value1: "kwatch",
		Value2: msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	responseBody, err := i.sender.Send(ctx, transport.Request{
		Provider: i.Name(), URL: i.url, Body: body,
		ContentType: "application/json",
	})
	if err != nil {
		return err
	}
	if len(responseBody) == 0 {
		return nil
	}
	var response iftttResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return fmt.Errorf("ifttt returned invalid response")
	}
	if len(response.Errors) > 0 {
		return fmt.Errorf("ifttt response reported errors")
	}
	return nil
}
