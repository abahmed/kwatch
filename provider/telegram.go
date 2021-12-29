package provider

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
)

const (
	telegramAPIURL = "https://api.telegram.org/bot%s/sendMessage"
)

type telegram struct {
	token  string
	chatId string
}

// NewTelegram returns a new Telegram object
func NewTelegram(token string, chatId string) Provider {
	// object validation
	if len(token) == 0 {
		logrus.Warnf("initializing telegram with empty token")
	} else if len(chatId) == 0 {
		logrus.Warnf("initializing telegram with empty chat_id")
	} else {
		logrus.Infof("initializing telegram with token  %s and chat_id %s", token, chatId)
	}

	// returns a new telegram object
	return &telegram{
		token:  token,
		chatId: chatId,
	}
}

// Name returns name of the provider
func (t *telegram) Name() string {
	return "Telegram"
}

// SendEvent sends event to the provider
func (t *telegram) SendEvent(e *event.Event) error {
	logrus.Debugf("sending to telegram event: %v", e)

	// validate telegram token and chat Id
	err, _ := validateTelegram(t)
	if err != nil {
		return err
	}
	reqBody := buildRequestBodyTelegram(e, t.chatId, "")
	return sendByTelegramApi(reqBody, t)
}

// SendMessage sends text message to the provider
func (t *telegram) SendMessage(msg string) error {
	logrus.Debugf("sending to telegram msg: %s", msg)

	// validate telegram token and chat Id
	err, _ := validateTelegram(t)
	if err != nil {
		return err
	}

	reqBody := buildRequestBodyTelegram(new(event.Event), t.chatId, msg)
	return sendByTelegramApi(reqBody, t)
}

func buildRequestBodyTelegram(e *event.Event, chatId string, customMsg string) string {
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
			"An alert for Name: *%s*  Container: *%s* Namespace: *%s*  has been triggered:\\n—\\n Logs: *%s* \\n Events: *%s* ",
			e.Name,
			e.Container,
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

func validateTelegram(t *telegram) (error, bool) {
	if len(t.token) == 0 {
		return errors.New("token key is empty"), false
	}
	if len(t.chatId) == 0 {
		return errors.New("chat id is empty"), false
	}
	return nil, true
}

func sendByTelegramApi(reqBody string, t *telegram) error {

	client := &http.Client{}
	buffer := bytes.NewBuffer([]byte(reqBody))
	url := fmt.Sprintf(telegramAPIURL, t.token)

	request, err := http.NewRequest(http.MethodPost, url, buffer)
	response, err := client.Do(request)
	if err != nil || response.StatusCode > 202 {
		return err
	}

	return nil
}
