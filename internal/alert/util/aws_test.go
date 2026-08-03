package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSignAWSV4Headers(t *testing.T) {
	assert := assert.New(t)

	headers, err := SignAWSV4(
		"AKIA123", "secret", "us-east-1", "ses", "POST",
		"https://email.us-east-1.amazonaws.com/",
		[]byte("Action=SendEmail&Version=2010-12-01"))
	assert.Nil(err)
	assert.NotNil(headers["X-Amz-Date"])
	assert.Contains(headers["Authorization"], "AWS4-HMAC-SHA256")
	assert.Contains(headers["Authorization"], "Credential=AKIA123/")
	assert.Contains(headers["Authorization"], "/us-east-1/ses/aws4_request")
	assert.Contains(headers["Authorization"], "SignedHeaders=content-type;host;x-amz-date")
	assert.Contains(headers["Authorization"], "Signature=")
}

func TestSignAWSV4Deterministic(t *testing.T) {
	assert := assert.New(t)

	h1, err := SignAWSV4("k", "s", "us-east-1", "sns", "POST",
		"https://sns.us-east-1.amazonaws.com/", []byte("a=1"))
	assert.Nil(err)
	h2, err := SignAWSV4("k", "s", "us-east-1", "sns", "POST",
		"https://sns.us-east-1.amazonaws.com/", []byte("a=1"))
	assert.Nil(err)
	h3, err := SignAWSV4("k", "s", "eu-west-1", "sns", "POST",
		"https://sns.eu-west-1.amazonaws.com/", []byte("a=1"))
	assert.Nil(err)
	h4, err := SignAWSV4("k", "other", "us-east-1", "sns", "POST",
		"https://sns.us-east-1.amazonaws.com/", []byte("a=1"))
	assert.Nil(err)

	assert.Equal(h1, h2)
	assert.NotEqual(h1, h3)
	assert.NotEqual(h1, h4)
}

func TestSignAWSV4InvalidURL(t *testing.T) {
	assert := assert.New(t)

	_, err := SignAWSV4("k", "s", "us-east-1", "sns", "POST", "h ttp://bad", nil)
	assert.NotNil(err)
}
