package email

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomail "gopkg.in/mail.v2"

	"github.com/abahmed/kwatch/internal/event"
)

func mockedSend(m ...*gomail.Message) error {
	return nil
}

func TestEmailEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewEmail(map[string]interface{}{}, "dev")
	assert.Nil(c)
}

func TestEmailInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"from": "test@test.com",
	}
	c := NewEmail(configMap, "dev")
	assert.Nil(c)

	configMap = map[string]interface{}{
		"from": "test@test.com",
		"to":   "test12@test.com",
	}
	c = NewEmail(configMap, "dev")
	assert.Nil(c)

	configMap = map[string]interface{}{
		"from":     "test@test.com",
		"to":       "test12@test.com",
		"password": "testPassword",
	}
	c = NewEmail(configMap, "dev")
	assert.Nil(c)

	configMap = map[string]interface{}{
		"from":     "test@test.com",
		"to":       "test12@test.com",
		"password": "testPassword",
		"host":     "chat.google.com",
	}
	c = NewEmail(configMap, "dev")
	assert.Nil(c)

	configMap = map[string]interface{}{
		"from":     "test@test.com",
		"to":       "test12@test.com",
		"password": "testPassword",
		"host":     "chat.google.com",
		"port":     "string",
	}
	c = NewEmail(configMap, "dev")
	assert.Nil(c)

	configMap = map[string]interface{}{
		"from":     "test@test.com",
		"to":       "test12@test.com",
		"password": "testPassword",
		"host":     "chat.google.com",
		"port":     "65539",
	}
	c = NewEmail(configMap, "dev")
	assert.Nil(c)
}

func TestEmail(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"from":     "test@test.com",
		"to":       "test12@test.com",
		"password": "testPassword",
		"host":     "chat.google.com",
		"port":     "587",
	}
	c := NewEmail(configMap, "dev")
	assert.NotNil(c)

	assert.Equal(c.Name(), "Email")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"from":     "test@test.com",
		"to":       "test12@test.com",
		"password": "testPassword",
		"host":     "chat.google.com",
		"port":     "587",
	}
	c := NewEmail(configMap, "dev")
	assert.NotNil(c)

	c.send = mockedSend
	assert.Nil(c.SendMessage(context.Background(), "test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"from":     "test@test.com",
		"to":       "test12@test.com",
		"password": "testPassword",
		"host":     "chat.google.com",
		"port":     "587",
	}
	c := NewEmail(configMap, "dev")
	assert.NotNil(c)

	c.send = mockedSend
	ev := event.Event{
		NodeName:      "test-node",
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs:          "test\ntestlogs",
		Events: "event1-event2-event3-event1-event2-event3-event1-event2-" +
			"event3\nevent5\nevent6-event8-event11-event12",
	}
	assert.Nil(c.SendEvent(context.Background(), &ev))
}

func TestSendEventHonorsCancellationBeforeSMTPDial(t *testing.T) {
	configMap := map[string]interface{}{
		"from": "test@test.com", "to": "to@test.com",
		"password": "password", "host": "127.0.0.1", "port": "587",
	}
	c := NewEmail(configMap, "dev")
	require.NotNil(t, c)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := c.SendEvent(ctx, &event.Event{Reason: "test"})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSendEventCancellationClosesSMTPConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			defer conn.Close()
			_, _ = io.Copy(io.Discard, conn)
		}
	}()

	c := NewEmail(map[string]interface{}{
		"from": "test@test.com", "to": "to@test.com",
		"password": "password", "host": "127.0.0.1",
		"port": fmt.Sprint(listener.Addr().(*net.TCPAddr).Port),
	}, "dev")
	require.NotNil(t, c)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = c.SendEvent(ctx, &event.Event{Reason: "test"})
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
