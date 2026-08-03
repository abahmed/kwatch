package email

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	gomail "gopkg.in/mail.v2"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
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
		klog.InfoS("initializing email with an empty from")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing email with an empty to")
		return nil
	}

	password, ok := config["password"].(string)
	if !ok || len(password) == 0 {
		klog.InfoS("initializing email with an empty password")
		return nil
	}

	host, ok := config["host"].(string)
	if !ok || len(host) == 0 {
		klog.InfoS("initializing email with an empty host")
		return nil
	}

	port, ok := config["port"].(string)
	if !ok || len(port) == 0 {
		klog.InfoS("initializing email with an empty port number")
		return nil
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil {
		klog.InfoS("initializing email with an invalid port number", "error", err)
		return nil
	}

	if portNumber > math.MaxUint16 {
		klog.InfoS("initializing email with an invalid range for port number")
		return nil
	}

	d := gomail.NewDialer(host, portNumber, from, password)
	d.StartTLSPolicy = gomail.MandatoryStartTLS

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

func (e *Email) UsesEventDelivery() {}

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
	subject := "⛑ Kwatch alert"
	if ev.ContainerName != "" {
		subject = fmt.Sprintf("⛑ Kwatch detected a crash in pod %s", ev.ContainerName)
	} else if ev.PodName != "" {
		subject = fmt.Sprintf("⛑ Kwatch detected a crash in pod %s", ev.PodName)
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("Reason: %s", util.OrDefault(ev.Reason, "unknown")))

	if ev.PodName != "" {
		parts = append(parts, fmt.Sprintf("Pod: %s", ev.PodName))
	}
	if ev.ContainerName != "" {
		parts = append(parts, fmt.Sprintf("Container: %s", ev.ContainerName))
	}
	if ev.Namespace != "" {
		parts = append(parts, fmt.Sprintf("Namespace: %s", ev.Namespace))
	}
	if ev.NodeName != "" {
		parts = append(parts, fmt.Sprintf("Node: %s", ev.NodeName))
	}
	if e.appCfg.ClusterName != "" {
		parts = append(parts, fmt.Sprintf("Cluster: %s", e.appCfg.ClusterName))
	}

	body := strings.Join(parts, "\n")

	if ev.IncludeLogs {
		logs := strings.TrimSpace(ev.Logs)
		if len(logs) > 0 {
			body += "\n\nLogs:\n" + logs
		}
	}

	if ev.IncludeEvents {
		events := strings.TrimSpace(ev.Events)
		if len(events) > 0 {
			body += "\n\nEvents:\n" + events
		}
	}

	return subject, body
}
