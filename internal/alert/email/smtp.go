package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	gomail "gopkg.in/mail.v2"
)

const smtpTimeout = 10 * time.Second

type smtpConfig struct {
	host     string
	port     int
	username string
	password string
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

	if config.port == 465 {
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

	if config.port != 465 {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: config.host}); err != nil {
			return smtpContextError(ctx, err)
		}
	}
	if config.username != "" {
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
	if err := client.Quit(); err != nil {
		return smtpContextError(ctx, err)
	}
	return ctx.Err()
}

func smtpContextError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
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
