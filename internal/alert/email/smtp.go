package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

	gomail "gopkg.in/mail.v2"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

const smtpTimeout = 10 * time.Second

// tlsRequired uses implicit TLS on port 465 and STARTTLS elsewhere.
// tlsNone is for trusted in-cluster relays and never sends credentials.
const (
	tlsRequired = "required"
	tlsNone     = "none"
)

type smtpConfig struct {
	host     string
	port     int
	username string
	password string
	tlsMode  string
}

// sendSMTP keeps the SMTP connection tied to the caller's context. gomail's
// DialAndSend has a finite timeout but no context cancellation path, so a
// canceled delivery could otherwise outlive the delivery generation.
func sendSMTP(
	ctx context.Context,
	config smtpConfig,
	from string,
	to string,
	message *gomail.Message,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dialer := net.Dialer{Timeout: smtpTimeout}
	conn, err := dialer.DialContext(
		ctx, "tcp", net.JoinHostPort(config.host, fmt.Sprint(config.port)),
	)
	if err != nil {
		return err
	}
	stop := make(chan struct{})
	go closeOnSMTPContext(ctx, conn, stop)
	defer close(stop)
	defer func() { _ = conn.Close() }()

	if config.port == 465 && config.tlsMode != tlsNone {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: config.host})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return err
		}
		conn = tlsConn
	}
	client, err := smtp.NewClient(conn, config.host)
	if err != nil {
		return smtpContextError(ctx, err)
	}
	defer func() { _ = client.Close() }()

	if config.port != 465 && config.tlsMode != tlsNone {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: config.host}); err != nil {
			return smtpContextError(ctx, err)
		}
	}
	if config.password != "" {
		if err := client.Auth(smtp.PlainAuth(
			"", config.username, config.password, config.host,
		)); err != nil {
			return smtpContextError(ctx, err)
		}
	}
	if err := client.Mail(from); err != nil {
		return smtpContextError(ctx, err)
	}
	for _, recipient := range strings.Split(to, ",") {
		if err := client.Rcpt(strings.TrimSpace(recipient)); err != nil {
			return smtpContextError(ctx, err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return smtpContextError(ctx, err)
	}
	if _, err := message.WriteTo(writer); err != nil {
		_ = writer.Close()
		return smtpContextError(ctx, err)
	}
	if err := writer.Close(); err != nil {
		return smtpContextError(ctx, err)
	}
	// The server accepted the message when DATA was closed. Reporting a
	// late cancellation or a failed QUIT now would make delivery retry and
	// send the same mail twice.
	if err := client.Quit(); err != nil {
		klog.V(4).InfoS("SMTP QUIT failed after the message was accepted",
			"component", "delivery", "provider", "Email", "error", err)
	}
	return nil
}

// smtpContextError reports the caller's cancellation when there is one,
// and otherwise the SMTP failure, classified for retry.
func smtpContextError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return classifySMTP(err)
}

// classifySMTP marks a 5xx reply (bad credentials, unknown recipient,
// rejected sender) permanent, since sending the same mail again gets the
// same answer. A 4xx reply is the server asking to try later, and any
// other failure (a dropped connection) stays retryable. 552 (mailbox full
// or message size exceeded) is the one 5xx that clears when the recipient
// frees space, so it is retried like the 4xx replies.
func classifySMTP(err error) error {
	var reply *textproto.Error
	if errors.As(err, &reply) && reply.Code >= 500 &&
		reply.Code != 552 {
		return transport.Permanent(err)
	}
	return err
}

func closeOnSMTPContext(
	ctx context.Context,
	conn net.Conn,
	stop <-chan struct{},
) {
	timer := time.NewTimer(smtpTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		_ = conn.Close()
	case <-timer.C:
		_ = conn.Close()
	case <-stop:
	}
}
