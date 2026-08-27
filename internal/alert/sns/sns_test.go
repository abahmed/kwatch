package sns

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

	c := NewSns(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestSns(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessKeyId":     "AKIA123",
		"secretAccessKey": "test",
		"topicArn":        "arn:aws:sns:us-east-1:123456789012:kwatch",
	}
	c := NewSns(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "SNS")
	assert.Equal(c.url, "https://sns.us-east-1.amazonaws.com/")
}

func TestSnsCustomRegion(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessKeyId":     "AKIA123",
		"secretAccessKey": "test",
		"region":          "us-east-1",
		"targetArn":       "arn:aws:sns:us-east-1:123456789012:kwatch",
	}
	c := NewSns(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.url, "https://sns.us-east-1.amazonaws.com/")
}

func TestSnsInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewSns(map[string]interface{}{"secretAccessKey": "s", "topicArn": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewSns(map[string]interface{}{"accessKeyId": "a", "topicArn": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewSns(map[string]interface{}{"accessKeyId": "a", "secretAccessKey": "s"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotAuth string
	var gotDate string
	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotDate = r.Header.Get("X-Amz-Date")
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.Write([]byte(`<PublishResponse></PublishResponse>`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessKeyId":     "AKIA123",
		"secretAccessKey": "test",
		"topicArn":        "arn:aws:sns:us-east-1:123456789012:kwatch",
		"subject":         "kwatch alert",
	}
	c := NewSns(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("hello"))
	assert.Contains(gotAuth, "AWS4-HMAC-SHA256")
	assert.Contains(gotAuth, "Credential=AKIA123/")
	assert.Contains(gotAuth, "SignedHeaders=content-type;host;x-amz-date")
	assert.Contains(gotAuth, "Signature=")
	assert.NotEmpty(gotDate)
	assert.Contains(gotBody, "Action=Publish")
	assert.Contains(gotBody, "Version=2010-03-31")
	assert.Contains(gotBody, "TopicArn=arn%3Aaws%3Asns%3Aus-east-1%3A123456789012%3Akwatch")
	assert.Contains(gotBody, "Message=hello")
	assert.Contains(gotBody, "Subject=kwatch+alert")
}

func TestSendMessageTargetArn(t *testing.T) {
	assert := assert.New(t)

	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.Write([]byte(`<PublishResponse></PublishResponse>`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessKeyId":     "AKIA123",
		"secretAccessKey": "test",
		"targetArn":       "arn:aws:sns:us-east-1:123456789012:endpoint",
	}
	c := NewSns(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("hello"))
	assert.Contains(gotBody, "TargetArn=arn%3Aaws%3Asns%3Aus-east-1%3A123456789012%3Aendpoint")
	assert.NotContains(gotBody, "TopicArn")
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
		"topicArn":        "arn:aws:sns:us-east-1:123456789012:kwatch",
	}
	c := NewSns(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.NotNil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "OOMKILLED")
			w.Write([]byte(`<PublishResponse></PublishResponse>`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessKeyId":     "AKIA123",
		"secretAccessKey": "test",
		"topicArn":        "arn:aws:sns:us-east-1:123456789012:kwatch",
	}
	c := NewSns(configMap, &config.App{ClusterName: "dev"})
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
		"topicArn":        "arn:aws:sns:us-east-1:123456789012:kwatch",
	}
	c := NewSns(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
