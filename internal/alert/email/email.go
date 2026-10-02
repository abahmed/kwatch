package email

import (
	"context"
	"math"
	"strconv"
	"strings"

	gomail "gopkg.in/mail.v2"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/notification"
)

type Email struct {
	from string
	to   string
	send func(m ...*gomail.Message) error

	smtpConfig smtpConfig

	// reference for general app configuration
	clusterName string
}

// NewEmail returns new email instance
func NewEmail(config map[string]interface{}, clusterName string) *Email {
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

	// An internal relay may accept mail without authentication, so the
	// password is optional; username defaults to the from address.
	password, _ := config["password"].(string)
	username, _ := config["username"].(string)
	if username == "" && password != "" {
		username = from
	}
	tlsMode, _ := config["tls"].(string)
	if tlsMode == "" {
		tlsMode = tlsRequired
	}
	if tlsMode != tlsRequired && tlsMode != tlsNone {
		klog.InfoS("initializing email with an invalid tls mode",
			"tls", tlsMode)
		return nil
	}
	if tlsMode == tlsNone && password != "" {
		klog.InfoS("email tls none cannot be used with a password")
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

	return &Email{
		from: from,
		to:   to,
		smtpConfig: smtpConfig{
			host: host, port: portNumber,
			username: username, password: password,
			tlsMode: tlsMode,
		},
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (e *Email) Name() string {
	return "Email"
}

// SendIncident mails one incident message: the Short lead is the
// subject and the narrative Note, plus any recent output, is the body.
func (e *Email) SendIncident(
	ctx context.Context, msg notification.Message,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m := gomail.NewMessage()
	m.SetHeader("From", e.from)
	m.SetHeader("To", strings.Split(e.to, ",")...)
	m.SetHeader("Subject", msg.MailSubject())
	if e.clusterName != "" {
		m.SetHeader("X-Kwatch-Cluster", e.clusterName)
	}
	if msg.Key != "" {
		m.SetHeader("X-Kwatch-Alert-Key", msg.AlertKey(e.clusterName))
	}
	m.SetBody("text/plain", msg.MailBody())

	if e.send != nil {
		return e.send(m)
	}
	return sendSMTP(ctx, e.smtpConfig, e.from, e.to, m)
}

// SendMessage mails a plain operator message as a notice.
func (e *Email) SendMessage(ctx context.Context, s string) error {
	return e.SendIncident(ctx, notification.Notice(s))
}
