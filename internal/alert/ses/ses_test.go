package ses

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

func TestEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewSes(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestSes(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessKeyId":     "AKIA123",
		"secretAccessKey": "test",
		"from":            "kwatch@example.com",
		"to":              "ops@example.com",
	}
	c := NewSes(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "SES")
	assert.Equal(c.url, "https://email.us-east-1.amazonaws.com/")
}

func TestSesCustomRegion(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessKeyId":     "AKIA123",
		"secretAccessKey": "test",
		"region":          "us-east-1",
		"from":            "kwatch@example.com",
		"to":              "ops@example.com",
	}
	c := NewSes(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.url, "https://email.us-east-1.amazonaws.com/")
}

func TestSesMultiTo(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessKeyId":     "AKIA123",
		"secretAccessKey": "test",
		"from":            "kwatch@example.com",
		"to":              "ops@example.com, dev@example.com",
	}
	c := NewSes(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Len(c.to, 2)
}

func TestSesInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewSes(map[string]interface{}{"secretAccessKey": "s", "from": "f", "to": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewSes(map[string]interface{}{"accessKeyId": "a", "from": "f", "to": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewSes(map[string]interface{}{"accessKeyId": "a", "secretAccessKey": "s", "to": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewSes(map[string]interface{}{"accessKeyId": "a", "secretAccessKey": "s", "from": "f"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotAuth string
	var gotDate string
	var gotCT string
	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotDate = r.Header.Get("X-Amz-Date")
			gotCT = r.Header.Get("Content-Type")
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.Write([]byte(`<SendEmailResponse></SendEmailResponse>`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessKeyId":     "AKIA123",
		"secretAccessKey": "test",
		"from":            "kwatch@example.com",
		"to":              "ops@example.com",
	}
	c := NewSes(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("hello"))
	assert.Contains(gotCT, "application/x-www-form-urlencoded")
	assert.NotEmpty(gotDate)
	assert.Contains(gotAuth, "AWS4-HMAC-SHA256")
	assert.Contains(gotAuth, "Credential=AKIA123/")
	assert.Contains(gotAuth, "SignedHeaders=content-type;host;x-amz-date")
	assert.Contains(gotAuth, "Signature=")
	assert.Contains(gotBody, "Action=SendEmail")
	assert.Contains(gotBody, "Version=2010-12-01")
	assert.Contains(gotBody, "Source=kwatch%40example.com")
	assert.Contains(gotBody, "Destination.ToAddresses.member.1=ops%40example.com")
	assert.Contains(gotBody, "Message.Subject.Data=kwatch+alert")
	assert.Contains(gotBody, "Message.Body.Text.Data=hello")
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessKeyId":     "AKIA123",
		"secretAccessKey": "test",
		"from":            "kwatch@example.com",
		"to":              "ops@example.com",
	}
	c := NewSes(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.NotNil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "OOMKILLED")
			w.Write([]byte(`<SendEmailResponse></SendEmailResponse>`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessKeyId":     "AKIA123",
		"secretAccessKey": "test",
		"from":            "kwatch@example.com",
		"to":              "ops@example.com",
	}
	c := NewSes(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	ev := event.Event{
		PodName:   "test-pod",
		Namespace: "default",
		Reason:    "OOMKILLED",
	}
	assert.Nil(c.SendEvent(&ev))
}

func TestInvalidHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessKeyId":     "AKIA123",
		"secretAccessKey": "test",
		"from":            "kwatch@example.com",
		"to":              "ops@example.com",
	}
	c := NewSes(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
