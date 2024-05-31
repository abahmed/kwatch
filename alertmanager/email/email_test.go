package email

import (
	"testing"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/event"
	"github.com/stretchr/testify/assert"
	gomail "gopkg.in/mail.v2"
)

func mockedSend(m ...*gomail.Message) error {
	return nil
}

func TestEmailEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewEmail(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestEmailInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"from": "test@test.com",
	}
	c := NewEmail(configMap, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	configMap = map[string]interface{}{
		"from": "test@test.com",
		"to":   "test12@test.com",
	}
	c = NewEmail(configMap, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	configMap = map[string]interface{}{
		"from":     "test@test.com",
		"to":       "test12@test.com",
		"password": "testPassword",
	}
	c = NewEmail(configMap, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	configMap = map[string]interface{}{
		"from":     "test@test.com",
		"to":       "test12@test.com",
		"password": "testPassword",
		"host":     "chat.google.com",
	}
	c = NewEmail(configMap, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	configMap = map[string]interface{}{
		"from":     "test@test.com",
		"to":       "test12@test.com",
		"password": "testPassword",
		"host":     "chat.google.com",
		"port":     "string",
	}
	c = NewEmail(configMap, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	configMap = map[string]interface{}{
		"from":     "test@test.com",
		"to":       "test12@test.com",
		"password": "testPassword",
		"host":     "chat.google.com",
		"port":     "65539",
	}
	c = NewEmail(configMap, &config.App{ClusterName: "dev"})
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
	c := NewEmail(configMap, &config.App{ClusterName: "dev"})
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
	c := NewEmail(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	c.send = mockedSend
	assert.Nil(c.SendMessage("test"))
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
	c := NewEmail(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	c.send = mockedSend
	ev := event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs:          "test\ntestlogs",
		Events: "event1-event2-event3-event1-event2-event3-event1-event2-" +
			"event3\nevent5\nevent6-event8-event11-event12",
	}
	assert.Nil(c.SendEvent(&ev))
}
