package email

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomail "gopkg.in/mail.v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// authSMTP is a local SMTP server that advertises the given AUTH
// mechanisms and records the login it was given.
type authSMTP struct {
	mechanisms string
	mu         sync.Mutex
	commands   []string
	user, pass string
	accepted   bool
}

func (a *authSMTP) start(t *testing.T) int {
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
		a.serve(conn)
	}()
	return listener.Addr().(*net.TCPAddr).Port
}

func decoded(line string) string {
	raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(line))
	return string(raw)
}

func (a *authSMTP) serve(conn net.Conn) {
	reader := bufio.NewReader(conn)
	say := func(line string) { _, _ = fmt.Fprint(conn, line+"\r\n") }
	say("220 test ready")
	inData := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)
		a.mu.Lock()
		if !inData {
			a.commands = append(a.commands, strings.Fields(upper + " x")[0])
		}
		a.mu.Unlock()
		switch {
		case inData && line == ".":
			inData = false
			a.mu.Lock()
			a.accepted = true
			a.mu.Unlock()
			say("250 queued")
		case inData:
		case strings.HasPrefix(upper, "EHLO"):
			say("250-test")
			say("250 AUTH " + a.mechanisms)
		case upper == "AUTH LOGIN":
			say("334 " + base64.StdEncoding.EncodeToString(
				[]byte("Username:")))
			user, _ := reader.ReadString('\n')
			say("334 " + base64.StdEncoding.EncodeToString(
				[]byte("Password:")))
			pass, _ := reader.ReadString('\n')
			a.mu.Lock()
			a.user, a.pass = decoded(user), decoded(pass)
			a.mu.Unlock()
			say("235 ok")
		case strings.HasPrefix(upper, "AUTH"):
			say("504 5.7.4 Unrecognized authentication type")
		case upper == "DATA":
			inData = true
			say("354 go ahead")
		case upper == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

func authMail() *gomail.Message {
	m := gomail.NewMessage()
	m.SetHeader("From", "kwatch@example.test")
	m.SetHeader("To", "ops@example.test")
	m.SetBody("text/plain", "hello")
	return m
}

// A server that offers only AUTH LOGIN (Exchange, some relays) answers
// 504 to PLAIN, a permanent failure for every mail. LOGIN is used there.
func TestSMTPUsesLoginWhenPlainIsNotOffered(t *testing.T) {
	server := &authSMTP{mechanisms: "LOGIN XOAUTH2"}
	config := smtpConfig{host: "127.0.0.1", port: server.start(t),
		username: "bot@example.test", password: "s3cret",
		tlsMode: tlsNone}

	err := sendSMTP(t.Context(), config, "kwatch@example.test",
		[]string{"ops@example.test"}, authMail())

	require.NoError(t, err)
	server.mu.Lock()
	defer server.mu.Unlock()
	assert.Equal(t, "bot@example.test", server.user)
	assert.Equal(t, "s3cret", server.pass)
	assert.True(t, server.accepted)
}

// PLAIN stays the choice wherever it is offered.
func TestSMTPKeepsPlainWhenItIsOffered(t *testing.T) {
	server := &authSMTP{mechanisms: "LOGIN PLAIN"}
	config := smtpConfig{host: "127.0.0.1", port: server.start(t),
		username: "bot@example.test", password: "s3cret",
		tlsMode: tlsNone}

	err := sendSMTP(t.Context(), config, "kwatch@example.test",
		[]string{"ops@example.test"}, authMail())

	// The fake refuses PLAIN, which shows PLAIN was what was tried.
	require.Error(t, err)
	assert.True(t, transport.IsPermanent(err))
	assert.Empty(t, server.user)
}

// LOGIN sends the password as it is, so it keeps PLAIN's guard.
func TestSMTPLoginRefusesAnUnencryptedRemoteServer(t *testing.T) {
	auth := &loginAuth{username: "u", password: "p", host: "mail.test"}
	_, _, err := auth.Start(&smtp.ServerInfo{Name: "mail.test"})
	require.Error(t, err)
	_, _, err = auth.Start(&smtp.ServerInfo{Name: "other.test", TLS: true})
	require.Error(t, err)
	mechanism, _, err := auth.Start(
		&smtp.ServerInfo{Name: "mail.test", TLS: true})
	require.NoError(t, err)
	assert.Equal(t, "LOGIN", mechanism)
}
