package email

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/event"
	"github.com/abahmed/kwatch/util"
	"github.com/sirupsen/logrus"
	gomail "gopkg.in/mail.v2"
)

type Email struct {
	from string
	to   string
	send func(m ...*gomail.Message) error
}

// NewEmail returns new email instance
func NewEmail(config map[string]string) *Email {
	from, ok := config["from"]
	if !ok || len(from) == 0 {
		logrus.Warnf("initializing email with an empty from")
		return nil
	}

	to, ok := config["to"]
	if !ok || len(to) == 0 {
		logrus.Warnf("initializing email with an empty to")
		return nil
	}

	password, ok := config["password"]
	if !ok || len(password) == 0 {
		logrus.Warnf("initializing email with an empty password")
		return nil
	}

	host, ok := config["host"]
	if !ok || len(host) == 0 {
		logrus.Warnf("initializing email with an empty host")
		return nil
	}

	port, ok := config["port"]
	if !ok || len(port) == 0 {
		logrus.Warnf("initializing email with an empty port number")
		return nil
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil {
		logrus.Warnf("initializing email with an invalid port number: %s", err)
		return nil
	}

	if portNumber > math.MaxUint16 {
		logrus.Warnf("initializing email with an invalid range for port number")
		return nil
	}

	d := gomail.NewDialer(host, portNumber, from, password)

	return &Email{
		from: from,
		to:   to,
		send: d.DialAndSend,
	}
}

// Name returns name of the provider
func (e *Email) Name() string {
	return "Email"
}

// SendEvent sends event to the provider
func (e *Email) SendEvent(event *event.Event) error {
	subject, body := buildMessageSubjectAndBody(event)

	m := gomail.NewMessage()
	m.SetHeader("From", e.from)
	m.SetHeader("To", strings.Split(e.to, ",")...)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)

	return e.send(m)
}

// SendMessage sends text message to the provider
func (e *Email) SendMessage(s string) error {
	return nil
}

func buildMessageSubjectAndBody(e *event.Event) (string, string) {
	eventsText := "No events captured"
	logsText := "No logs captured"

	// add events part if it exists
	events := strings.TrimSpace(e.Events)
	if len(events) > 0 {
		eventsText = util.JsonEscape(e.Events)
	}

	// add logs part if it exists
	logs := strings.TrimSpace(e.Logs)
	if len(logs) > 0 {
		logsText = util.JsonEscape(e.Logs)
	}

	subject := fmt.Sprintf("⛑ Kwatch detected a crash in pod %s ", e.Container)
	body := fmt.Sprintf(
		"An alert for Name: *%s*  Container: *%s* Namespace: *%s*  "+
			"has been triggered:\\n—\\n "+
			"Logs: *%s* \\n "+
			"Events: *%s* ",
		e.Name,
		e.Container,
		e.Namespace,
		logsText,
		eventsText,
	)
	return subject, body
}
