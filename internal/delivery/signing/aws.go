package signing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
	"time"
)

const (
	awsAlgorithm   = "AWS4-HMAC-SHA256"
	awsTerminator  = "aws4_request"
	awsContentType = "application/x-www-form-urlencoded"
)

// Credentials are AWS access keys. SessionToken is set for temporary
// credentials such as STS or IAM role sessions.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

// SignAWSV4At signs a request using the supplied time.
func SignAWSV4At(
	creds Credentials,
	region, service, method, rawURL string,
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

	canonicalHeaders, signedHeaders := canonicalHeaderBlock(
		creds.SessionToken, host, amzDate)

	canonicalRequest := strings.Join([]string{
		method,
		path,
		"",
		canonicalHeaders,
		signedHeaders,
		sha256Hex(body),
	}, "\n")

	scope := dateStamp + "/" + region + "/" + service + "/" + awsTerminator
	stringToSign := strings.Join([]string{
		awsAlgorithm,
		amzDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := buildSigningKey(
		creds.SecretAccessKey, dateStamp, region, service,
	)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authorization := awsAlgorithm + " Credential=" + creds.AccessKeyID +
		"/" + scope + ", SignedHeaders=" + signedHeaders +
		", Signature=" + signature

	return requestHeaders(amzDate, authorization, creds.SessionToken), nil
}

// canonicalHeaderBlock returns the canonical header text and the matching
// signed-header list. The session token is signed only when present.
func canonicalHeaderBlock(
	sessionToken, host, amzDate string,
) (canonical, signed string) {
	canonical = "content-type:" + awsContentType +
		"\nhost:" + host + "\nx-amz-date:" + amzDate + "\n"
	signed = "content-type;host;x-amz-date"
	if sessionToken != "" {
		canonical += "x-amz-security-token:" + sessionToken + "\n"
		signed += ";x-amz-security-token"
	}
	return canonical, signed
}

// requestHeaders are the headers the caller adds to the signed request.
func requestHeaders(
	amzDate, authorization, sessionToken string,
) map[string]string {
	headers := map[string]string{
		"X-Amz-Date":    amzDate,
		"Authorization": authorization,
	}
	if sessionToken != "" {
		headers["X-Amz-Security-Token"] = sessionToken
	}
	return headers
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
