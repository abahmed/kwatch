package sns

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/signing"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const (
	snsServiceName   = "sns"
	snsURLFormat     = "https://sns.%s.amazonaws.com/"
	defaultSNSRegion = "us-east-1"
)

type Sns struct {
	sender          transport.Sender
	url             string
	region          string
	accessKeyID     string
	secretAccessKey string
	topicArn        string
	targetArn       string
	subject         string
	now             func() time.Time

	clusterName string
}

// NewSns returns a new Sns object

func NewSns(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Sns {
	accessKeyID, ok := config["accessKeyId"].(string)
	if !ok || len(accessKeyID) == 0 {
		klog.InfoS("initializing sns with empty accessKeyId")
		return nil
	}

	secretAccessKey, ok := config["secretAccessKey"].(string)
	if !ok || len(secretAccessKey) == 0 {
		klog.InfoS("initializing sns with empty secretAccessKey")
		return nil
	}

	topicArn, _ := config["topicArn"].(string)
	targetArn, _ := config["targetArn"].(string)
	if len(topicArn) == 0 && len(targetArn) == 0 {
		klog.InfoS("initializing sns with empty topicArn or targetArn")
		return nil
	}

	region := defaultSNSRegion
	if r, ok := config["region"].(string); ok && len(r) > 0 {
		region = r
	}

	subject, _ := config["subject"].(string)

	klog.InfoS("initializing sns", "region", region, "topicArn", topicArn)

	return &Sns{
		sender:          transport.NewSender(dependencies),
		url:             fmt.Sprintf(snsURLFormat, region),
		region:          region,
		accessKeyID:     accessKeyID,
		secretAccessKey: secretAccessKey,
		topicArn:        topicArn,
		targetArn:       targetArn,
		subject:         subject,
		clusterName:     clusterName,
		now:             dependencies.Now,
	}
}

// Name returns name of the provider
func (s *Sns) Name() string {
	return "SNS"
}

// SendEvent sends event to the provider
func (s *Sns) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(s.clusterName, "")
	return s.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (s *Sns) SendMessage(ctx context.Context, msg string) error {
	form := url.Values{}
	form.Set("Action", "Publish")
	form.Set("Version", "2010-03-31")
	if len(s.targetArn) > 0 {
		form.Set("TargetArn", s.targetArn)
	} else {
		form.Set("TopicArn", s.topicArn)
	}
	form.Set("Message", msg)
	if len(s.subject) > 0 {
		form.Set("Subject", s.subject)
	}

	body := []byte(form.Encode())
	contentType := "application/x-www-form-urlencoded"

	headers, err := signing.SignAWSV4At(
		s.accessKeyID, s.secretAccessKey, s.region, snsServiceName,
		"POST", s.url, body, s.now())
	if err != nil {
		return err
	}

	_, err = s.sender.Send(ctx, transport.Request{
		Provider: s.Name(), URL: s.url, Body: body,
		ContentType: contentType, Headers: headers,
	})
	return err
}
