package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"sync/atomic"
	"time"

	gomail "gopkg.in/mail.v2"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/cloudmeta"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// smtpTimeout bounds the dial and each wait on the server (the idle
// time); smtpSessionTimeout bounds the whole session, so a slow server can
// finish a large DATA without a hung one holding the delivery for long.
// smtpDataEndTimeout is the wait for the reply to the end of DATA: a
// server that scans the message for spam may take far longer than any
// other step (RFC 5321 allows 10 minutes). The session limit is never
// below that wait plus one idle step.
const (
	smtpTimeout        = 10 * time.Second
	smtpDataEndTimeout = 2 * time.Minute
	smtpSessionTimeout = smtpDataEndTimeout + 2*smtpTimeout
)

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
	// tlsConfig is the application's TLS policy; nil means defaults.
	tlsConfig *tls.Config
	// idleTimeout, dataEndTimeout and sessionTimeout override the
	// defaults when set.
	idleTimeout    time.Duration
	dataEndTimeout time.Duration
	sessionTimeout time.Duration
	// dialer is the dialer to guard; nil means one with smtpTimeout.
	dialer *net.Dialer
}

// dial connects to the SMTP server. Like the HTTP providers it never
// reaches a cloud metadata service: a host that names one is refused
// before any I/O, and the guarded dialer refuses a name that resolves
// to one.
func (c smtpConfig) dial(ctx context.Context) (net.Conn, error) {
	if cloudmeta.Host(c.host) {
		return nil, transport.Permanent(transport.ErrMetadataAddress)
	}
	base := c.dialer
	if base == nil {
		base = &net.Dialer{Timeout: smtpTimeout}
	}
	return transport.GuardDialer(base).DialContext(ctx, "tcp",
		net.JoinHostPort(c.host, fmt.Sprint(c.port)))
}

func (c smtpConfig) idle() time.Duration {
	if c.idleTimeout > 0 {
		return c.idleTimeout
	}
	return smtpTimeout
}

func (c smtpConfig) dataEnd() time.Duration {
	if c.dataEndTimeout > 0 {
		return c.dataEndTimeout
	}
	return smtpDataEndTimeout
}

// session is the whole-session limit, at least the wait for the end of
// DATA plus one idle step, so the longer wait is never cut by it.
func (c smtpConfig) session() time.Duration {
	limit := smtpSessionTimeout
	if c.sessionTimeout > 0 {
		limit = c.sessionTimeout
	}
	return max(limit, c.dataEnd()+c.idle())
}

// idleConn restarts a watchdog on every read and write that closes the
// connection when one step of the conversation takes longer than idle, so
// the limit is per step and not for the whole session.
type idleConn struct {
	net.Conn
	idle  atomic.Int64
	timer *time.Timer
}

func newIdleConn(conn net.Conn, idle time.Duration) *idleConn {
	c := &idleConn{Conn: conn}
	c.idle.Store(int64(idle))
	c.timer = time.AfterFunc(idle, func() { _ = conn.Close() })
	return c
}

// allow sets the limit for each following step and restarts the watchdog.
func (c *idleConn) allow(idle time.Duration) {
	c.idle.Store(int64(idle))
	c.timer.Reset(idle)
}

func (c *idleConn) Read(p []byte) (int, error) {
	c.timer.Reset(time.Duration(c.idle.Load()))
	return c.Conn.Read(p)
}

func (c *idleConn) Write(p []byte) (int, error) {
	c.timer.Reset(time.Duration(c.idle.Load()))
	return c.Conn.Write(p)
}

// clientTLS returns a TLS config for host, never sharing mutable state.
func (c smtpConfig) clientTLS() *tls.Config {
	cfg := &tls.Config{}
	if c.tlsConfig != nil {
		cfg = c.tlsConfig.Clone()
	}
	cfg.ServerName = c.host
	return cfg
}

// sendSMTP keeps the SMTP connection tied to the caller's context. gomail's
// DialAndSend has a finite timeout but no context cancellation path, so a
// canceled delivery could otherwise outlive the delivery generation.
func sendSMTP(
	ctx context.Context,
	config smtpConfig,
	from string,
	to []string,
	message *gomail.Message,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	conn, err := config.dial(ctx)
	if err != nil {
		return err
	}
	stop := make(chan struct{})
	go closeOnSMTPContext(ctx, conn, config.session(), stop)
	defer close(stop)
	defer func() { _ = conn.Close() }()
	watched := newIdleConn(conn, config.idle())
	defer watched.timer.Stop()
	conn = watched

	if config.port == 465 && config.tlsMode != tlsNone {
		tlsConn := tls.Client(conn, config.clientTLS())
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
		if err := client.StartTLS(config.clientTLS()); err != nil {
			return smtpContextError(ctx, err)
		}
	}
	if config.password != "" {
		if err := client.Auth(config.auth(client)); err != nil {
			return smtpContextError(ctx, err)
		}
	}
	if err := client.Mail(from); err != nil {
		return smtpContextError(ctx, err)
	}
	if err := addRecipients(ctx, client, to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return smtpContextError(ctx, err)
	}
	if _, err := message.WriteTo(writer); err != nil {
		_ = writer.Close()
		return smtpContextError(ctx, err)
	}
	// The server may scan the message before it answers the end of DATA.
	watched.allow(config.dataEnd())
	if err := writer.Close(); err != nil {
		return afterBody(ctx, err)
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

// afterBody classifies a failure at the end of DATA. The whole body is
// written, so a lost connection or a timeout may have come after the
// server accepted the mail: it is reported as permanent, since sending it
// again could deliver it twice. A reply from the server is a clear answer
// and is classified as usual.
func afterBody(ctx context.Context, err error) error {
	var reply *textproto.Error
	if ctx.Err() != nil || errors.As(err, &reply) {
		return smtpContextError(ctx, err)
	}
	return transport.Permanent(fmt.Errorf("SMTP message may have been "+
		"delivered (no answer after the full body was sent; not "+
		"retrying to avoid a duplicate): %w", err))
}

// addRecipients sends RCPT TO for each address. A recipient the server
// refuses is skipped and logged, so one bad address does not lose the mail
// for the others. It fails, with the first refusal classified for retry,
// only when no recipient was accepted or the connection broke.
func addRecipients(
	ctx context.Context, client *smtp.Client, to []string,
) error {
	var first error
	accepted := 0
	for _, recipient := range to {
		err := client.Rcpt(recipient)
		if err == nil {
			accepted++
			continue
		}
		var reply *textproto.Error
		if !errors.As(err, &reply) {
			return smtpContextError(ctx, err)
		}
		klog.InfoS("SMTP server rejected a recipient",
			"component", "delivery", "provider", "Email",
			"recipient", recipient, "code", reply.Code)
		if first == nil {
			first = err
		}
	}
	if accepted == 0 {
		return smtpContextError(ctx, first)
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

// closeOnSMTPContext closes the connection when the caller cancels or the
// whole session passes limit.
func closeOnSMTPContext(
	ctx context.Context,
	conn net.Conn,
	limit time.Duration,
	stop <-chan struct{},
) {
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		_ = conn.Close()
	case <-timer.C:
		_ = conn.Close()
	case <-stop:
	}
}
