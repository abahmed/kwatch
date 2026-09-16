package dingtalk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

const (
	dingTalkAPIURL = "https://oapi.dingtalk.com/robot/send?access_token=%s"
)

type dingResponse struct {
	Errcode int    `json:"errcode"`
	Errmsg  string `json:"errmsg"`
}

type DingTalk struct {
	sender      transport.Sender
	accessToken string
	secret      string
	url         string
	title       string
	clockSource clock.Clock

	// reference for general app configuration
	clusterName string
}

// NewDingTalk returns new DingTalk instance

func NewDingTalk(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *DingTalk {
	accessToken, ok := config["accessToken"].(string)
	if !ok || len(accessToken) == 0 {
		klog.InfoS("initializing dingtalk with empty access token")
		return nil
	}

	klog.InfoS("initializing dingtalk with access token")

	title, _ := config["title"].(string)
	secret, _ := config["secret"].(string)

	return &DingTalk{
		sender:      transport.NewSender(dependencies),
		accessToken: accessToken,
		url:         dingTalkAPIURL,
		title:       title,
		secret:      secret,
		clusterName: clusterName,
		clockSource: clock.Require(dependencies.Clock),
	}
}

// Name returns name of the provider
func (d *DingTalk) Name() string {
	return "DingTalk"
}

// SendEvent sends event to the provider
func (d *DingTalk) SendEvent(ctx context.Context, e *event.Event) error {
	title := d.title
	if len(title) == 0 {
		title = constant.DefaultTitle
	}

	msg := e.FormatMarkdown(d.clusterName, "", "")

	payload := struct {
		MsgType  string `json:"msgtype"`
		Markdown struct {
			Title string `json:"title"`
			Text  string `json:"text"`
		} `json:"markdown"`
	}{
		MsgType: "markdown",
	}
	payload.Markdown.Title = title
	payload.Markdown.Text = msg

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return d.sendAPI(ctx, string(bodyBytes))
}

// SendMessage sends text message to the provider
func (d *DingTalk) SendMessage(ctx context.Context, msg string) error {
	payload := struct {
		MsgType string `json:"msgtype"`
		Text    struct {
			Content string `json:"content"`
		} `json:"text"`
	}{
		MsgType: "text",
	}
	payload.Text.Content = msg

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return d.sendAPI(ctx, string(bodyBytes))
}

// SendIncident implements delivery.ThreadProvider.
// It renders the incident using the Report model and PlaintextRenderer,
// producing a context-adaptive text message.
func (d *DingTalk) SendIncident(
	ctx context.Context,
	inc *model.Incident,
	action model.IncidentAction,
) error {
	return d.SendIncidentWithInsight(ctx, inc, action, nil)
}

// SendIncidentWithInsight implements delivery.InsightThreadProvider, so the
// diagnosis — likely cause, impact, recent changes — is rendered rather than
// dropped on the way to this provider.
func (d *DingTalk) SendIncidentWithInsight(
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
		d.clusterName,
		d.clockSource,
	)
	if text == "" {
		return nil
	}
	return d.SendMessage(ctx, text)
}

func (d *DingTalk) sendAPI(ctx context.Context, msg string) error {
	url := fmt.Sprintf(d.url, d.accessToken)
	if len(d.secret) != 0 {
		url += getSignatureAt(d.secret, d.clockSource.Now())
	}
	data, err := d.sender.Send(ctx, transport.Request{
		Provider: "DingTalk", URL: url, Body: []byte(msg),
	})
	if err != nil {
		return err
	}

	// DingTalk answers 200 to a rejected message and reports the failure in
	// the body instead. Some of those codes are transient (130101 is its
	// frequency limit), so the error stays retryable.
	var dr dingResponse
	if err := json.Unmarshal(data, &dr); err != nil {
		return err
	}
	if dr.Errcode != 0 {
		return fmt.Errorf(
			"call to ding talk alert rejected (errcode %d): %s",
			dr.Errcode,
			string(data),
		)
	}
	return nil
}

func getSignatureAt(secret string, now time.Time) string {
	timeStr := fmt.Sprintf("%d", now.UnixNano()/1e6)

	sign := fmt.Sprintf("%s\n%s", timeStr, secret)
	signData := computeHmacSha256(sign, secret)
	encodeURL := url.QueryEscape(signData)

	return fmt.Sprintf("&timestamp=%s&sign=%s", timeStr, encodeURL)
}

func computeHmacSha256(message string, secret string) string {
	key := []byte(secret)
	h := hmac.New(sha256.New, key)
	h.Write([]byte(message))

	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
