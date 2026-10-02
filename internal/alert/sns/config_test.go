package sns

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSnsConfig(t *testing.T) {
	deps := providertest.NewRecorder(t).Dependencies()
	c := NewSns(map[string]interface{}{
		"accessKeyId": "AKIA123", "secretAccessKey": "s", "topicArn": "t",
	}, "dev", deps)
	assert.Equal(t, "SNS", c.Name())
	assert.Equal(t, "https://sns.us-east-1.amazonaws.com/", c.url)

	c = NewSns(map[string]interface{}{
		"accessKeyId": "a", "secretAccessKey": "s", "targetArn": "t",
		"region": "eu-west-1",
	}, "dev", deps)
	assert.Equal(t, "https://sns.eu-west-1.amazonaws.com/", c.url)
}

func TestSnsRejectsIncompleteConfig(t *testing.T) {
	deps := providertest.NewRecorder(t).Dependencies()
	for name, config := range map[string]map[string]interface{}{
		"empty":     {},
		"no key id": {"secretAccessKey": "s", "topicArn": "t"},
		"no secret": {"accessKeyId": "a", "topicArn": "t"},
		"no target": {"accessKeyId": "a", "secretAccessKey": "s"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Nil(t, NewSns(config, "dev", deps))
		})
	}
}
