package email

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// fakeSMTP is a plain SMTP server for one session. It records the
// envelope and can refuse recipients or answer slowly.
type fakeSMTP struct {
	mu       sync.Mutex
	mailFrom string
	rcpts    []string
	data     string
	accepted bool
	// reject returns a reply line for a recipient to refuse, or "".
	reject func(rcpt string) string
	// delay is how long each reply waits.
	delay time.Duration
	// endDelay is the extra wait before the reply to the end of DATA.
	endDelay time.Duration
}

func (f *fakeSMTP) start(t *testing.T) (port string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		f.serve(conn)
	}()
	return fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
}

func (f *fakeSMTP) serve(conn net.Conn) {
	reader := bufio.NewReader(conn)
	reply := func(line string) {
		time.Sleep(f.delay)
		_, _ = fmt.Fprint(conn, line+"\r\n")
	}
	reply("220 test ready")
	inData := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		switch {
		case inData && line == ".":
			inData = false
			f.mu.Lock()
			f.accepted = true
			f.mu.Unlock()
			time.Sleep(f.endDelay)
			reply("250 queued")
		case inData:
			f.mu.Lock()
			f.data += line + "\n"
			f.mu.Unlock()
		default:
			inData = f.command(line, reply)
			if strings.EqualFold(line, "QUIT") {
				return
			}
		}
	}
}

// command answers one command and reports whether DATA began.
func (f *fakeSMTP) command(line string, reply func(string)) bool {
	upper := strings.ToUpper(line)
	switch {
	case strings.HasPrefix(upper, "MAIL FROM:"):
		f.mu.Lock()
		f.mailFrom = line[len("MAIL FROM:"):]
		f.mu.Unlock()
		reply("250 ok")
	case strings.HasPrefix(upper, "RCPT TO:"):
		rcpt := line[len("RCPT TO:"):]
		if f.reject != nil && f.reject(rcpt) != "" {
			reply(f.reject(rcpt))
			break
		}
		f.mu.Lock()
		f.rcpts = append(f.rcpts, rcpt)
		f.mu.Unlock()
		reply("250 ok")
	case upper == "DATA":
		reply("354 go ahead")
		return true
	case upper == "QUIT":
		reply("221 bye")
	case strings.HasPrefix(upper, "EHLO"):
		reply("250 test")
	default:
		reply("250 ok")
	}
	return false
}

func relayConfig(port, from, to string) map[string]interface{} {
	return map[string]interface{}{
		"from": from, "to": to, "host": "127.0.0.1", "port": port,
		"tls": "none",
	}
}

func TestEmailEnvelopeUsesBareAddresses(t *testing.T) {
	f := &fakeSMTP{}
	port := f.start(t)
	c := NewEmail(relayConfig(port, "Kwatch Bot <kwatch@example.test>",
		`Ops <ops@example.test>, "Doe, Jane" <jane@example.test>`),
		"dev", transport.Dependencies{})
	require.NotNil(t, c)

	require.NoError(t, c.SendIncident(t.Context(), providertest.Announce()))
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, "<kwatch@example.test>", f.mailFrom)
	assert.Equal(t, []string{"<ops@example.test>", "<jane@example.test>"},
		f.rcpts)
	assert.Contains(t, f.data, "From: \"Kwatch Bot\" <kwatch@example.test>")
}

func TestEmailUsernameDefaultsToBareAddress(t *testing.T) {
	cfg := relayConfig("587", "Kwatch <kwatch@example.test>", "a@example.test")
	cfg["tls"] = "required"
	cfg["password"] = "pw"
	c := NewEmail(cfg, "dev", transport.Dependencies{})
	require.NotNil(t, c)
	assert.Equal(t, "kwatch@example.test", c.smtpConfig.username)
}

func TestEmailRejectsUnparsableAddresses(t *testing.T) {
	for _, tc := range []struct{ name, from, to string }{
		{"from", "not an address", "a@example.test"},
		{"from unclosed", "Kwatch <kwatch@example.test", "a@example.test"},
		{"to", "k@example.test", "a@example.test, nope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewEmail(relayConfig("25", tc.from, tc.to), "dev",
				transport.Dependencies{})
			assert.Nil(t, c)
		})
	}
}

func TestEmailSkipsRejectedRecipient(t *testing.T) {
	f := &fakeSMTP{reject: func(r string) string {
		if strings.Contains(r, "bad@") {
			return "550 no such user"
		}
		return ""
	}}
	port := f.start(t)
	c := NewEmail(relayConfig(port, "k@example.test",
		"bad@example.test, ok@example.test"), "dev",
		transport.Dependencies{})
	require.NotNil(t, c)

	require.NoError(t, c.SendIncident(t.Context(), providertest.Announce()))
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, []string{"<ok@example.test>"}, f.rcpts)
	assert.True(t, f.accepted, "the mail goes to the accepted recipient")
}

func TestEmailFailsWhenEveryRecipientIsRejected(t *testing.T) {
	for _, tc := range []struct {
		reply     string
		permanent bool
	}{{"550 no such user", true}, {"451 try later", false}} {
		t.Run(tc.reply, func(t *testing.T) {
			f := &fakeSMTP{reject: func(string) string { return tc.reply }}
			port := f.start(t)
			c := NewEmail(relayConfig(port, "k@example.test",
				"a@example.test, b@example.test"), "dev",
				transport.Dependencies{})
			require.NotNil(t, c)

			err := c.SendIncident(t.Context(), providertest.Announce())
			require.Error(t, err)
			assert.Equal(t, tc.permanent, transport.IsPermanent(err))
			f.mu.Lock()
			defer f.mu.Unlock()
			assert.False(t, f.accepted)
		})
	}
}

func TestEmailSessionTimeoutIsNotOneTimerForEverything(t *testing.T) {
	// Eight replies at 60ms take about half a second, longer than the
	// 300ms idle limit, but no single wait is.
	f := &fakeSMTP{delay: 60 * time.Millisecond}
	port := f.start(t)
	c := NewEmail(relayConfig(port, "k@example.test", "a@example.test"),
		"dev", transport.Dependencies{})
	require.NotNil(t, c)
	c.smtpConfig.idleTimeout = 300 * time.Millisecond
	c.smtpConfig.sessionTimeout = 5 * time.Second

	require.NoError(t, c.SendIncident(t.Context(), providertest.Announce()))
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.True(t, f.accepted)
}

func TestEmailHungServerIsCutOffAtIdleTimeout(t *testing.T) {
	f := &fakeSMTP{delay: time.Second}
	port := f.start(t)
	c := NewEmail(relayConfig(port, "k@example.test", "a@example.test"),
		"dev", transport.Dependencies{})
	require.NotNil(t, c)
	c.smtpConfig.idleTimeout = 100 * time.Millisecond

	start := time.Now()
	err := c.SendIncident(t.Context(), providertest.Announce())
	require.Error(t, err)
	assert.Less(t, time.Since(start), 900*time.Millisecond)
}

func TestEmailAcceptsSemicolonSeparatedRecipients(t *testing.T) {
	f := &fakeSMTP{}
	port := f.start(t)
	c := NewEmail(relayConfig(port, "k@example.test",
		`a@example.test; "Doe; Jane" <b@example.test>,c@example.test`),
		"dev", transport.Dependencies{})
	require.NotNil(t, c)

	require.NoError(t, c.SendIncident(t.Context(), providertest.Announce()))
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, []string{"<a@example.test>", "<b@example.test>",
		"<c@example.test>"}, f.rcpts)
}
