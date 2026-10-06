package feishu

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

// feiShuTextLimit keeps the card within Feishu's request size limit.
const feiShuTextLimit = 30000

// defaultTitle is the card header when the operator sets none.
const defaultTitle = "kwatch"

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

	if !transport.ValidEndpoint(webhook) {
		klog.InfoS("initializing feishu with an invalid webhook",
			"setting", "webhook")
		return nil
	}

	klog.InfoS("initializing Fei Shu with webhook configured")

	title, _ := config["title"].(string)
	if strings.TrimSpace(title) == "" {
		// Feishu may reject a card whose header title is empty.
		title = defaultTitle
	}
	secret, _ := config["secret"].(string)

	return &FeiShu{
		sender:      transport.NewSender(dependencies),
		webhook:     webhook,
		title:       title,
		secret:      secret,
		clusterName: clusterName,
		clockSource: clock.Require(dependencies.Clock),
	}

}

// Name returns name of the provider
func (f *FeiShu) Name() string {
	return "Fei Shu"
}

// SendIncident sends the incident narrative as the card's markdown
// element, with the application output in a code block after it.
func (f *FeiShu) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	text := safetext.NoteWithOutput(
		m.NoteText(), m.Output, "\n\n", feiShuTextLimit)
	body, err := f.buildRequestBodyFeiShu(text)
	if err != nil {
		return err
	}
	return f.sendByFeiShuApi(ctx, body)
}

// rateLimitCodes are the documented frequency-limit codes; every other
// body error is a permanent rejection.
var rateLimitCodes = map[int]bool{11232: true}

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
		err := fmt.Errorf("feishu request failed with code %d", response.Code)
		if rateLimitCodes[response.Code] {
			return &ratelimit.Error{Provider: "Feishu",
				StatusCode: ratelimit.InBodyStatus, Err: err}
		}
		return transport.Permanent(err)
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
