package email

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/event"
	"github.com/abahmed/kwatch/util"
	"github.com/sirupsen/logrus"
	gomail "gopkg.in/mail.v2"
)

type Email struct {
	from string
	to   string
	send func(m ...*gomail.Message) error

	// reference for general app configuration
	appCfg *config.App
}

// NewEmail returns new email instance
func NewEmail(config map[string]interface{}, appCfg *config.App) *Email {
	from, ok := config["from"].(string)
	if !ok || len(from) == 0 {
		logrus.Warnf("initializing email with an empty from")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		logrus.Warnf("initializing email with an empty to")
		return nil
	}

	password, ok := config["password"].(string)
	if !ok || len(password) == 0 {
		logrus.Warnf("initializing email with an empty password")
		return nil
	}

	host, ok := config["host"].(string)
	if !ok || len(host) == 0 {
		logrus.Warnf("initializing email with an empty host")
		return nil
	}

	port, ok := config["port"].(string)
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
		from:   from,
		to:     to,
		send:   d.DialAndSend,
		appCfg: appCfg,
	}
}

// Name returns name of the provider
func (e *Email) Name() string {
	return "Email"
}

// SendEvent sends event to the provider
func (e *Email) SendEvent(event *event.Event) error {
	subject, body := e.buildMessageSubjectAndBody(event)

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

func (e *Email) buildMessageSubjectAndBody(
	ev *event.Event) (string, string) {
	eventsText := "No events captured"
	logsText := "No logs captured"

	// add events part if it exists
	events := strings.TrimSpace(ev.Events)
	if len(events) > 0 {
		eventsText = util.JsonEscape(ev.Events)
	}

	// add logs part if it exists
	logs := strings.TrimSpace(ev.Logs)
	if len(logs) > 0 {
		logsText = util.JsonEscape(ev.Logs)
	}

	subject := fmt.Sprintf("⛑ Kwatch detected a crash in pod %s ", ev.ContainerName)
	body := fmt.Sprintf(
		"An alert for cluster: *%s* Name: *%s*  Container: *%s* "+
			"Namespace: *%s*  "+
			"has been triggered:\\n—\\n "+
			"Logs: *%s* \\n "+
			"Events: *%s* ",
		e.appCfg.ClusterName,
		ev.PodName,
		ev.ContainerName,
		ev.Namespace,
		logsText,
		eventsText,
	)
	return subject, body
}
