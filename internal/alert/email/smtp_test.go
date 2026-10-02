package email

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// serveSMTP answers one plain SMTP session and calls onQuit when the
// client sends QUIT, before the 221 reply.
func serveSMTP(
	t *testing.T, onQuit func(),
) (port string, done <-chan struct{}) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		answerSMTP(conn, onQuit)
	}()
	return fmt.Sprint(listener.Addr().(*net.TCPAddr).Port), finished
}

func answerSMTP(conn net.Conn, onQuit func()) {
	reader := bufio.NewReader(conn)
	reply := func(line string) { _, _ = fmt.Fprint(conn, line+"\r\n") }
	reply("220 test ready")
	inData := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		command := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case inData && command == ".":
			inData = false
			reply("250 queued")
		case inData:
		case strings.HasPrefix(command, "EHLO"):
			reply("250 test")
		case command == "DATA":
			inData = true
			reply("354 go ahead")
		case command == "QUIT":
			onQuit()
			reply("221 bye")
			return
		default:
			reply("250 ok")
		}
	}
}

func TestEmailAcceptedMessageIgnoresLateCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	port, done := serveSMTP(t, cancel)

	c := NewEmail(map[string]interface{}{
		"from": "kwatch@example.test", "to": "ops@example.test",
		"host": "127.0.0.1", "port": port, "tls": "none",
	}, "dev")
	require.NotNil(t, c)

	err := c.SendIncident(ctx, providertest.Announce())
	require.NoError(t, err, "an accepted message must not be retried")
	<-done
}
