package signing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
)

const (
	awsAlgorithm   = "AWS4-HMAC-SHA256"
	awsTerminator  = "aws4_request"
	awsSignedHdrs  = "content-type;host;x-amz-date"
	awsContentType = "application/x-www-form-urlencoded"
)

// SignAWSV4 computes SigV4 authorization headers for an AWS Query/JSON API
// request. The body must be URL-encoded form data and the endpoint a plain
// https URL. It returns the X-Amz-Date and Authorization headers to send.
func SignAWSV4(
	accessKey, secretKey, region, service, method, rawURL string,
	body []byte,
) (map[string]string, error) {
	return SignAWSV4At(
		accessKey,
		secretKey,
		region,
		service,
		method,
		rawURL,
		body,
		clock.RealClock{}.Now(),
	)
}

// SignAWSV4At signs a request using the supplied time. Keeping the time at
// this boundary makes signatures deterministic without changing callers that
// use the production wrapper.
func SignAWSV4At(
	accessKey, secretKey, region, service, method, rawURL string,
	body []byte,
	now time.Time,
) (map[string]string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	host := parsed.Host
	path := parsed.Path
	if len(path) == 0 {
		path = "/"
	}

	now = now.UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	canonicalHeaders := "content-type:" + awsContentType +
		"\nhost:" + host + "\nx-amz-date:" + amzDate + "\n"

	canonicalRequest := strings.Join([]string{
		method,
		path,
		"",
		canonicalHeaders,
		awsSignedHdrs,
		sha256Hex(body),
	}, "\n")

	scope := dateStamp + "/" + region + "/" + service + "/" + awsTerminator
	stringToSign := strings.Join([]string{
		awsAlgorithm,
		amzDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := buildSigningKey(secretKey, dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authorization := awsAlgorithm + " Credential=" + accessKey + "/" + scope +
		", SignedHeaders=" + awsSignedHdrs + ", Signature=" + signature

	return map[string]string{
		"X-Amz-Date":    amzDate,
		"Authorization": authorization,
	}, nil
}

func buildSigningKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte(awsTerminator))
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
