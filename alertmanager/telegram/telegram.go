package telegram

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
)

const (
	telegramAPIURL = "https://api.telegram.org/bot%s/sendMessage"
)

type Telegram struct {
	token  string
	chatId string
	url    string

	// reference for general app configuration
	appCfg *config.App
}

// NewTelegram returns a new Telegram object
func NewTelegram(config map[string]interface{}, appCfg *config.App) *Telegram {
	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		logrus.Warnf("initializing telegram with empty token")
		return nil
	}

	chatId, ok := config["chatId"].(string)
	if !ok || len(chatId) == 0 {
		logrus.Warnf("initializing telegram with empty chat_id")
		return nil
	}

	logrus.Infof(
		"initializing telegram with token  %s and chat_id %s",
		token,
		chatId)

	// returns a new telegram object
	return &Telegram{
		token:  token,
		chatId: chatId,
		url:    telegramAPIURL,
		appCfg: appCfg,
	}
}

// Name returns name of the provider
func (t *Telegram) Name() string {
	return "Telegram"
}

// SendEvent sends event to the provider
func (t *Telegram) SendEvent(e *event.Event) error {
	logrus.Debugf("sending to telegram event: %v", e)

	reqBody := t.buildRequestBodyTelegram(e, t.chatId, "")
	return t.sendByTelegramApi(reqBody)
}

// SendMessage sends text message to the provider
func (t *Telegram) SendMessage(msg string) error {
	logrus.Debugf("sending to telegram msg: %s", msg)

	reqBody := t.buildRequestBodyTelegram(new(event.Event), t.chatId, msg)
	return t.sendByTelegramApi(reqBody)
}

func (t *Telegram) buildRequestBodyTelegram(
	e *event.Event,
	chatId string,
	customMsg string) string {
	eventsText := "No events captured"
	logsText := "No logs captured"

	// add events part if it exists
	events := strings.TrimSpace(e.Events)
	if len(events) > 0 {
		eventsText = e.Events
	}

	// add logs part if it exists
	logs := strings.TrimSpace(e.Logs)
	if len(logs) > 0 {
		logsText = e.Logs
	}

	// build text will be sent in the message
	txt := ""
	if len(customMsg) <= 0 {
		txt = fmt.Sprintf(
			"An alert for Cluster: *%s* Name: *%s*  "+
				"Container: *%s* "+
				"Namespace: *%s*  has been triggered:\\n—\\n "+
				"Logs: *%s* \\n "+
				"Events: *%s* ",
			t.appCfg.ClusterName,
			e.PodName,
			e.ContainerName,
			e.Namespace,
			logsText,
			eventsText,
		)
	} else {
		txt = customMsg
	}

	// build the message to be sent
	msg := fmt.Sprintf(
		"⛑ Kwatch detected a crash in pod \\n%s ",
		txt,
	)

	reqBody := fmt.Sprintf(
		`{"chat_id": "%s", "text": "%s", "parse_mode": "MARKDOWN"}`,
		chatId,
		msg,
	)
	return reqBody
}

func (t *Telegram) sendByTelegramApi(reqBody string) error {
	client := &http.Client{}
	buffer := bytes.NewBuffer([]byte(reqBody))
	url := fmt.Sprintf(t.url, t.token)

	request, err := http.NewRequest(http.MethodPost, url, buffer)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode > 202 {
		return fmt.Errorf(
			"call to telegram alert returned status code %d",
			response.StatusCode)
	}

	return nil
}
