package matrix

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/event"
	"github.com/abahmed/kwatch/util"
	"github.com/sirupsen/logrus"
)

type Matrix struct {
	serverURL      string
	accessToken    string
	internalRoomID string
	title          string
	text           string
}

// NewMatrix returns new Matrix instance
func NewMatrix(config map[string]string) *Matrix {
	serverURL, ok := config["serverurl"]
	if !ok || len(serverURL) == 0 {
		logrus.Warnf("initializing slack with empty serverURL")
		return nil
	}

	accessToken, ok := config["accesstoken"]
	if !ok || len(accessToken) == 0 {
		logrus.Warnf("initializing slack with empty accessToken")
		return nil
	}

	internalRoomID, ok := config["internalroomid"]
	if !ok || len(internalRoomID) == 0 {
		logrus.Warnf("initializing slack with empty internalroomid")
		return nil
	}

	return &Matrix{
		serverURL:      serverURL,
		accessToken:    accessToken,
		internalRoomID: internalRoomID,
		title:          config["title"],
		text:           config["text"],
	}
}

func (m *Matrix) Name() string {
	return "Matrix"
}

func (m *Matrix) SendMessage(msg string) error {
	return m.sendAPI(msg)
}

func (m *Matrix) SendEvent(e *event.Event) error {
	eventsText := constant.DefaultEvents
	logsText := constant.DefaultLogs

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
	// use custom title if it's provided, otherwise use default
	title := m.title
	if len(title) == 0 {
		title = constant.DefaultTitle
	}

	// use custom text if it's provided, otherwise use default
	text := m.text
	if len(text) == 0 {
		text = constant.DefaultText
	}

	msg := fmt.Sprintf(
		"%s<br/>"+
			"<b>Pod:</b> %s <br/>"+
			"<b>Container:</b> %s<br/>"+
			"<b>Namespace:</b> %s<br/>"+
			"<b>Events:</b><br/><blockquote>%s</blockquote><br/>"+
			"<b>Logs:</b> <br/><blockquote>%s</blockquote>",
		text,
		e.Name,
		e.Container,
		e.Namespace,
		eventsText,
		logsText,
	)

	return m.sendAPI(msg)
}

func (m *Matrix) sendAPI(formattedMsg string) error {
	plainMsg := stripHtmlRegex(formattedMsg)
	msg := fmt.Sprintf(`{
		"msgtype": "m.text",
		"format": "org.matrix.custom.html",
		"body": "%s",
		"formatted_body": "%s"
	}`,
		util.JsonEscape(plainMsg),
		util.JsonEscape(formattedMsg),
	)
	request, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf(
			"%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s"+
				"?access_token=%s",
			m.serverURL,
			url.PathEscape(m.internalRoomID),
			util.RandomString(24),
			url.QueryEscape(m.accessToken),
		),
		bytes.NewBuffer([]byte(msg)),
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

	if response.StatusCode > 399 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf(
			"call to matrix alert returned status code %d: %s",
			response.StatusCode,
			string(body))

	}

	return nil
}

// This method uses a regular expresion to remove HTML tags.
func stripHtmlRegex(s string) string {
	const regex = `<.*?>`
	r := regexp.MustCompile(regex)
	return r.ReplaceAllString(s, "")
}
