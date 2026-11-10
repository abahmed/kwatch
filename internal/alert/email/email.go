package email

import (
	"context"
	"math"
	"net/mail"
	"strconv"
	"strings"

	gomail "gopkg.in/mail.v2"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

type Email struct {
	// from and to keep the display names for the message headers; the
	// SMTP envelope uses the bare addresses.
	from *mail.Address
	to   []*mail.Address
	send func(m ...*gomail.Message) error

	smtpConfig smtpConfig

	// reference for general app configuration
	clusterName string
}

// NewEmail returns new email instance
//
// SMTP is not HTTP: the application's outbound proxy does not apply to it
// and mail is dialed directly. The application's TLS settings (extra root
// CAs and skip-verify) are taken from the shared client in dependencies.
func NewEmail(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Email {
	from, ok := config["from"].(string)
	if !ok || len(from) == 0 {
		klog.InfoS("initializing email with an empty from")
		return nil
	}
	fromAddr, err := mail.ParseAddress(from)
	if err != nil {
		klog.InfoS("initializing email with an invalid from address",
			"error", err)
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing email with an empty to")
		return nil
	}
	toAddrs, err := mail.ParseAddressList(commaSeparated(to))
	if err != nil {
		klog.InfoS("initializing email with an invalid to address",
			"error", err)
		return nil
	}

	// An internal relay may accept mail without authentication, so the
	// password is optional; username defaults to the from address.
	password, _ := config["password"].(string)
	username, _ := config["username"].(string)
	if username == "" && password != "" {
		username = fromAddr.Address
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
		from: fromAddr,
		to:   toAddrs,
		smtpConfig: smtpConfig{
			host: host, port: portNumber,
			username: username, password: password,
			tlsMode:   tlsMode,
			tlsConfig: dependencies.TLSConfig(),
		},
		clusterName: clusterName,
	}
}

// commaSeparated turns the ";" separators of a recipient list into ",",
// which is all mail.ParseAddressList accepts. A ";" inside a quoted
// display name or an angle-bracket address is part of it and stays.
func commaSeparated(list string) string {
	var out strings.Builder
	quoted, angle, escaped := false, false, false
	for _, r := range list {
		switch {
		case escaped:
			escaped = false
		case quoted && r == '\\':
			escaped = true
		case r == '"':
			quoted = !quoted
		case !quoted && r == '<':
			angle = true
		case !quoted && r == '>':
			angle = false
		case !quoted && !angle && r == ';':
			r = ','
		}
		out.WriteRune(r)
	}
	return out.String()
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
	m.SetHeader("From", m.FormatAddress(e.from.Address, e.from.Name))
	m.SetHeader("To", e.toHeader(m)...)
	m.SetHeader("Subject", msg.MailSubject())
	if e.clusterName != "" {
		m.SetHeader("X-Kwatch-Cluster", e.clusterName)
	}
	if msg.Key != "" {
		m.SetHeader("X-Kwatch-Alert-Key", msg.AlertKey(e.clusterName))
	}
	id, opening := threadIDs(msg, e.clusterName,
		idDomain(e.from.Address))
	if id != "" {
		m.SetHeader("Message-ID", id)
	}
	if opening != "" {
		m.SetHeader("In-Reply-To", opening)
		m.SetHeader("References", opening)
	}
	text, page := Bodies(msg)
	m.SetBody("text/plain", text)
	if page != "" {
		m.AddAlternative("text/html", page)
	}

	if e.send != nil {
		return e.send(m)
	}
	return sendSMTP(ctx, e.smtpConfig, e.from.Address, e.toEnvelope(), m)
}

// toHeader is the recipients as header values, with display names.
func (e *Email) toHeader(m *gomail.Message) []string {
	out := make([]string, 0, len(e.to))
	for _, a := range e.to {
		out = append(out, m.FormatAddress(a.Address, a.Name))
	}
	return out
}

// toEnvelope is the bare recipient addresses for RCPT TO.
func (e *Email) toEnvelope() []string {
	out := make([]string, 0, len(e.to))
	for _, a := range e.to {
		out = append(out, a.Address)
	}
	return out
}

// SendMessage mails a plain operator message as a notice.
func (e *Email) SendMessage(ctx context.Context, s string) error {
	return e.SendIncident(ctx, notification.Notice(s))
}
