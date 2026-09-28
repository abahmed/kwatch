package feishu

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

type FeiShu struct {
	sender  transport.Sender
	webhook string
	title   string
	// secret enables the bot's signature verification when set.
	secret string

	// reference for general app configuration
	clusterName string
	clockSource clock.Clock
}

type feiShuWebhookContent struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

type feiShuCardConfig struct {
	WideScreenMode bool `json:"wide_screen_mode"`
}

type feiShuHeaderTitle struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

type feiShuHeader struct {
	Title    feiShuHeaderTitle `json:"title"`
	Template string            `json:"template"`
}

type feiShuCard struct {
	Config   feiShuCardConfig       `json:"config"`
	Header   feiShuHeader           `json:"header"`
	Elements []feiShuWebhookContent `json:"elements"`
}

type feiShuRequestBody struct {
	Timestamp string     `json:"timestamp,omitempty"`
	Sign      string     `json:"sign,omitempty"`
	MsgType   string     `json:"msg_type"`
	Card      feiShuCard `json:"card"`
}

type feiShuResponse struct {
	Code int `json:"code"`
}

// NewFeiShu returns new feishu web bot instance

func NewFeiShu(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *FeiShu {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing Fei Shu with empty webhook url")
		return nil
	}

	klog.InfoS("initializing Fei Shu with webhook configured")

	title, _ := config["title"].(string)

	return &FeiShu{
		sender:      transport.NewSender(dependencies),
		webhook:     webhook,
		title:       title,
		clusterName: clusterName,
		clockSource: clock.Require(dependencies.Clock),
	}

}

// Name returns name of the provider
func (f *FeiShu) Name() string {
	return "Fei Shu"
}

// SendEvent sends event to the provider
func (f *FeiShu) SendEvent(ctx context.Context, e *event.Event) error {
	body, err := f.buildRequestBodyFeiShu(
		e.FormatMarkdown(f.clusterName, "", ""),
	)
	if err != nil {
		return err
	}
	return f.sendByFeiShuApi(ctx, body)
}

func (f *FeiShu) sendByFeiShuApi(
	ctx context.Context,
	reqBody string,
) error {
	body, err := f.sender.Send(ctx, transport.Request{
		Provider: "Feishu", URL: f.webhook, Body: []byte(reqBody),
	})
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	var response feiShuResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("feishu returned invalid response")
	}
	if response.Code != 0 {
		return fmt.Errorf("feishu request failed with code %d", response.Code)
	}
	return nil
}

// SendMessage sends text message to the provider
func (f *FeiShu) SendMessage(ctx context.Context, msg string) error {
	body, err := f.buildRequestBodyFeiShu(msg)
	if err != nil {
		return err
	}
	return f.sendByFeiShuApi(ctx, body)
}

// SendIncident implements delivery.ThreadProvider.
// It renders the incident using the Report model and PlaintextRenderer,
// producing a context-adaptive text message.
func (f *FeiShu) SendIncident(
	ctx context.Context,
	inc *model.Incident,
	action model.IncidentAction,
) error {
	return f.SendIncidentWithInsight(ctx, inc, action, nil)
}

// SendIncidentWithInsight implements delivery.InsightThreadProvider, so the
// diagnosis — likely cause, impact, recent changes — is rendered rather than
// dropped on the way to this provider.
func (f *FeiShu) SendIncidentWithInsight(
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
		f.clusterName,
		f.clockSource,
	)
	if text == "" {
		return nil
	}
	return f.SendMessage(ctx, text)
}

func (f *FeiShu) buildRequestBodyFeiShu(
	text string) (string, error) {
	body := feiShuRequestBody{
		MsgType: "interactive",
		Card: feiShuCard{
			Config: feiShuCardConfig{
				WideScreenMode: true,
			},
			Header: feiShuHeader{
				Title: feiShuHeaderTitle{
					Tag:     "plain_text",
					Content: f.title,
				},
				Template: "blue",
			},
			Elements: []feiShuWebhookContent{
				{
					Tag:     "markdown",
					Content: text,
				},
			},
		},
	}
	if f.secret != "" {
		body.Timestamp, body.Sign = feiShuSignature(
			f.secret, f.clockSource.Now(),
		)
	}
	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal feishu body: %w", err)
	}
	return string(jsonBytes), nil
}

// feiShuSignature implements Feishu custom bot signing: the HMAC-SHA256 key
// is "timestamp\nsecret" over an empty message, base64 encoded.
func feiShuSignature(secret string, now time.Time) (string, string) {
	timestamp := strconv.FormatInt(now.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(timestamp+"\n"+secret))
	return timestamp, base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
