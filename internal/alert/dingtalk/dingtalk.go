package dingtalk

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
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
	accessToken string
	secret      string
	url         string
	title       string

	// reference for general app configuration
	appCfg *config.App
}

// NewDingTalk returns new DingTalk instance
func NewDingTalk(config map[string]interface{}, appCfg *config.App) *DingTalk {
	accessToken, ok := config["accessToken"].(string)
	if !ok || len(accessToken) == 0 {
		klog.InfoS("initializing dingtalk with empty access token")
		return nil
	}

	klog.InfoS("initializing dingtalk with access token")

	title, _ := config["title"].(string)
	secret, _ := config["secret"].(string)

	return &DingTalk{
		accessToken: accessToken,
		url:         dingTalkAPIURL,
		title:       title,
		secret:      secret,
		appCfg:      appCfg,
	}
}

// Name returns name of the provider
func (d *DingTalk) Name() string {
	return "DingTalk"
}

// SendEvent sends event to the provider
func (d *DingTalk) SendEvent(e *event.Event) error {
	title := d.title
	if len(title) == 0 {
		title = constant.DefaultTitle
	}

	msg := e.FormatMarkdown(d.appCfg.ClusterName, "", "")

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

	return d.sendAPI(string(bodyBytes))
}

// SendMessage sends text message to the provider
func (d *DingTalk) SendMessage(msg string) error {
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

	return d.sendAPI(string(bodyBytes))
}

// SendIncident implements alert.ThreadProvider.
// It renders the incident using the Report model and PlaintextRenderer,
// producing a context-adaptive text message.
func (d *DingTalk) SendIncident(
	inc *model.Incident,
	action model.IncidentAction,
) error {
	return d.SendIncidentWithInsight(inc, action, nil)
}

// SendIncidentWithInsight implements alert.InsightThreadProvider, so the
// diagnosis — likely cause, impact, recent changes — is rendered rather than
// dropped on the way to this provider.
func (d *DingTalk) SendIncidentWithInsight(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	text := util.RenderIncidentWithInsight(
		inc,
		action,
		ins,
		message.NewPlainTextRenderer(),
		d.appCfg.ClusterName,
	)
	if text == "" {
		return nil
	}
	return d.SendMessage(text)
}

func (d *DingTalk) sendAPI(msg string) error {
	url := fmt.Sprintf(d.url, d.accessToken)
	if len(d.secret) != 0 {
		url += getSignature(d.secret)
	}
	data, err := util.Send(
		util.Request{Provider: "DingTalk", URL: url, Body: []byte(msg)},
	)
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

func getSignature(secret string) string {
	timeStr := fmt.Sprintf("%d", time.Now().UnixNano()/1e6)

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
