package email

import (
	"context"
	"net"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// dialLog is a dialer whose Control hook records every address it is
// about to connect to, after the metadata guard let it through.
type dialLog struct {
	mu   sync.Mutex
	seen []string
}

func (l *dialLog) dialer() *net.Dialer {
	return &net.Dialer{Timeout: smtpTimeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.seen = append(l.seen, address)
			return nil
		}}
}

func (l *dialLog) addresses() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.seen...)
}

// An SMTP host that is, or names, a cloud metadata service is refused
// for good before a connection is attempted.
func TestSMTPDialRefusesMetadataHosts(t *testing.T) {
	for _, host := range []string{"169.254.169.254", "fd00:ec2::254",
		"100.100.100.200", "metadata.google.internal", "64:ff9b::a9fe:a9fe",
	} {
		log := &dialLog{}
		config := smtpConfig{host: host, port: 25, dialer: log.dialer()}
		conn, err := config.dial(context.Background())
		if conn != nil {
			_ = conn.Close()
		}
		require.ErrorIs(t, err, transport.ErrMetadataAddress, host)
		assert.True(t, transport.IsPermanent(err), host)
		assert.Empty(t, log.addresses(), host)
	}
}

// A resolved metadata address is refused by the guarded dialer's hook,
// before the base dialer's own hook runs.
func TestSMTPDialGuardsTheResolvedAddress(t *testing.T) {
	log := &dialLog{}
	d := transport.GuardDialer(log.dialer())
	_, err := d.DialContext(context.Background(), "tcp",
		"169.254.169.254:25")
	require.ErrorIs(t, err, transport.ErrMetadataAddress)
	assert.Empty(t, log.addresses())
}

// A normal SMTP host is dialed, through the base dialer's own hook.
func TestSMTPDialAllowsANormalHost(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		if c, err := listener.Accept(); err == nil {
			_ = c.Close()
		}
	}()
	log := &dialLog{}
	port := listener.Addr().(*net.TCPAddr).Port
	config := smtpConfig{host: "127.0.0.1", port: port,
		dialer: log.dialer()}
	conn, err := config.dial(context.Background())
	require.NoError(t, err)
	_ = conn.Close()
	assert.Equal(t, []string{listener.Addr().String()}, log.addresses())
}
