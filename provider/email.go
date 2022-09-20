package provider

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
	gomail "gopkg.in/mail.v2"
)

type email struct {
	from     string
	password string
	host     string
	port     int
	to       string
}

// NewEmail returns new email instance
func NewEmail(config map[string]string) Provider {
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
	}

	return &email{
		from:     from,
		to:       to,
		password: password,
		host:     host,
		port:     portNumber,
	}
}

// Name returns name of the provider
func (e *email) Name() string {
	return "Email"
}

// SendEvent sends event to the provider
func (e *email) SendEvent(event *event.Event) error {
	subject, body := buildMessageSubjectAndBody(event)
	m := gomail.NewMessage()
	m.SetHeader("From", e.from)
	m.SetHeader("To", strings.Split(e.to, ",")...)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)
	d := gomail.NewDialer(e.host, e.port, e.from, e.password)
	return d.DialAndSend(m)
}

// SendMessage sends text message to the provider
func (e *email) SendMessage(s string) error {
	return nil
}

func buildMessageSubjectAndBody(e *event.Event) (string, string) {
	eventsText := "No events captured"
	logsText := "No logs captured"

	// add events part if it exists
	events := strings.TrimSpace(e.Events)
	if len(events) > 0 {
		eventsText = JsonEscape(e.Events)
	}

	// add logs part if it exists
	logs := strings.TrimSpace(e.Logs)
	if len(logs) > 0 {
		logsText = JsonEscape(e.Logs)
	}

	subject := fmt.Sprintf("⛑ Kwatch detected a crash in pod %s ", e.Container)
	body := fmt.Sprintf(
		"An alert for Name: *%s*  Container: *%s* Namespace: *%s*  has been triggered:\\n—\\n Logs: *%s* \\n Events: *%s* ",
		e.Name,
		e.Container,
		e.Namespace,
		logsText,
		eventsText,
	)
	return subject, body
}
