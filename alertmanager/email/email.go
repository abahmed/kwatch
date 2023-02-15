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
	from, ok := config["from"]
	fromString := fmt.Sprint(from)
	if !ok || len(fromString) == 0 {
		logrus.Warnf("initializing email with an empty from")
		return nil
	}

	to, ok := config["to"]
	toString := fmt.Sprint(to)
	if !ok || len(toString) == 0 {
		logrus.Warnf("initializing email with an empty to")
		return nil
	}

	password, ok := config["password"]
	passwordString := fmt.Sprint(password)
	if !ok || len(passwordString) == 0 {
		logrus.Warnf("initializing email with an empty password")
		return nil
	}

	host, ok := config["host"]
	hostString := fmt.Sprint(host)
	if !ok || len(hostString) == 0 {
		logrus.Warnf("initializing email with an empty host")
		return nil
	}

	port, ok := config["port"]
	portString := fmt.Sprint(port)
	if !ok || len(portString) == 0 {
		logrus.Warnf("initializing email with an empty port number")
		return nil
	}
	portNumber, err := strconv.Atoi(portString)
	if err != nil {
		logrus.Warnf("initializing email with an invalid port number: %s", err)
		return nil
	}

	if portNumber > math.MaxUint16 {
		logrus.Warnf("initializing email with an invalid range for port number")
		return nil
	}

	d := gomail.NewDialer(hostString, portNumber, fromString, passwordString)

	return &Email{
		from:   fromString,
		to:     toString,
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

	subject := fmt.Sprintf("⛑ Kwatch detected a crash in pod %s ", ev.Container)
	body := fmt.Sprintf(
		"An alert for cluster: *%s* Name: *%s*  Container: *%s* "+
			"Namespace: *%s*  "+
			"has been triggered:\\n—\\n "+
			"Logs: *%s* \\n "+
			"Events: *%s* ",
		e.appCfg.ClusterName,
		ev.Name,
		ev.Container,
		ev.Namespace,
		logsText,
		eventsText,
	)
	return subject, body
}
