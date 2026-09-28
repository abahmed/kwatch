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

	canonicalHeaders := "content-type:" + awsContentType +
		"\nhost:" + host + "\nx-amz-date:" + amzDate + "\n"
	signedHeaders := "content-type;host;x-amz-date"
	if creds.SessionToken != "" {
		canonicalHeaders += "x-amz-security-token:" +
			creds.SessionToken + "\n"
		signedHeaders += ";x-amz-security-token"
	}

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

	headers := map[string]string{
		"X-Amz-Date":    amzDate,
		"Authorization": authorization,
	}
	if creds.SessionToken != "" {
		headers["X-Amz-Security-Token"] = creds.SessionToken
	}
	return headers, nil
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
