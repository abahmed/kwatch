package dingtalk

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
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
		logrus.Warnf("initializing dingtalk with empty access token")
		return nil
	}

	logrus.Infof("initializing dingtalk with access token: %s", accessToken)

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

	// use custom title if it's provided, otherwise use default
	title := d.title
	if len(title) == 0 {
		title = constant.DefaultTitle
	}

	msg := e.FormatMarkdown(d.appCfg.ClusterName, "", "")

	body := fmt.Sprintf(`{
		"msgtype": "markdown",
		"markdown": { "title": "%s", "text: "%s" }
	}`, title, msg)

	return d.sendAPI(body)
}

// SendMessage sends text message to the provider
func (d *DingTalk) SendMessage(msg string) error {
	body := fmt.Sprintf(`{
		"msgtype": "text",
		"text": { "content": "%s"}
	}`, msg)
	return d.sendAPI(body)
}

func (d *DingTalk) sendAPI(msg string) error {
	buffer := bytes.NewBuffer([]byte(msg))

	url := fmt.Sprintf(d.url, d.accessToken)
	if len(d.secret) != 0 {
		url += getSignature(d.secret)
	}

	request, err := http.NewRequest(
		http.MethodPost,
		url,
		buffer,
	)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	data, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	var dr dingResponse
	err = json.Unmarshal(data, &dr)
	if err != nil {
		return err
	}
	if dr.Errcode != 0 {
		return fmt.Errorf(
			"call to ding talk alert returned status code %d: %s",
			response.StatusCode,
			string(data))
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
