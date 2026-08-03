package sns

import (
	"fmt"
	"net/url"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const (
	snsServiceName   = "sns"
	snsURLFormat     = "https://sns.%s.amazonaws.com/"
	defaultSNSRegion = "us-east-1"
)

type Sns struct {
	url             string
	region          string
	accessKeyID     string
	secretAccessKey string
	topicArn        string
	targetArn       string
	subject         string

	appCfg *config.App
}

// NewSns returns a new Sns object
func NewSns(config map[string]interface{}, appCfg *config.App) *Sns {
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
		url:             fmt.Sprintf(snsURLFormat, region),
		region:          region,
		accessKeyID:     accessKeyID,
		secretAccessKey: secretAccessKey,
		topicArn:        topicArn,
		targetArn:       targetArn,
		subject:         subject,
		appCfg:          appCfg,
	}
}

// Name returns name of the provider
func (s *Sns) Name() string {
	return "SNS"
}

// SendEvent sends event to the provider
func (s *Sns) SendEvent(e *event.Event) error {
	msg := e.FormatText(s.appCfg.ClusterName, "")
	return s.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (s *Sns) SendMessage(msg string) error {
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

	headers, err := util.SignAWSV4(
		s.accessKeyID, s.secretAccessKey, s.region, snsServiceName,
		"POST", s.url, body)
	if err != nil {
		return err
	}

	_, err = util.Post(s.Name(), s.url, body, contentType, headers)
	return err
}
