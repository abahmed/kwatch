package awscreds

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// ErrSessionTokenExpired is matched (errors.Is) by the error of a send
// AWS refused because the session token expired.
var ErrSessionTokenExpired = errors.New("AWS session token expired")

// ErrCredentialsRefused is matched by the retryable error of a send AWS
// refused with temporary credentials that kwatch drops and fetches
// again.
var ErrCredentialsRefused = errors.New("AWS refused the credentials")

// The error codes AWS answers a 403 with when the credentials, not the
// request, are wrong.
const (
	codeExpired       = "ExpiredToken"
	codeInvalidToken  = "InvalidClientTokenId"
	codeSignatureFail = "SignatureDoesNotMatch"
)

// refusalCodes maps each code AWS may send to the one above it means:
// the Query APIs (SNS, SES v1, STS) and the JSON ones name the same
// refusal differently.
var refusalCodes = []struct{ sent, code string }{
	{codeExpired, codeExpired},
	{"ExpiredTokenException", codeExpired},
	{codeInvalidToken, codeInvalidToken},
	{"UnrecognizedClientException", codeInvalidToken},
	{codeSignatureFail, codeSignatureFail},
}

// Refusals decides what a 403 about the credentials means for one
// provider. A refreshable source gets one retry with new credentials;
// after that, or for static keys, the refusal stays permanent.
type Refusals struct {
	mu      sync.Mutex
	retried bool
}

// Check returns the error a provider reports for its send. err and
// answer are what the sender returned; src signed the request.
func (r *Refusals) Check(
	provider string, src Source, err error, answer []byte,
) error {
	if err == nil {
		r.set(false)
		return nil
	}
	code := refusalCode(err, answer)
	refreshable := Refreshable(src)
	if code == "" || (code == codeSignatureFail && !refreshable) {
		return err
	}
	if refreshable && !r.set(true) {
		src.Invalidate()
		return fmt.Errorf("%s: %w (%s); fetching new ones and "+
			"retrying", provider, ErrCredentialsRefused, code)
	}
	if code == codeExpired {
		return transport.Permanent(&expiredError{
			provider: provider, static: !refreshable, err: err})
	}
	return err
}

// set stores retried and returns the value it had.
func (r *Refusals) set(retried bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	was := r.retried
	r.retried = retried
	return was
}

// refusalCode is the AWS error code of a 403 about the credentials, or
// "" for any other failure.
func refusalCode(err error, answer []byte) string {
	if status, ok := transport.StatusOf(err); !ok ||
		status != http.StatusForbidden {
		return ""
	}
	text := string(answer)
	for _, c := range refusalCodes {
		if hasCode(text, c.sent) {
			return c.code
		}
	}
	return ""
}

// hasCode reports whether an AWS error answer names code: as the XML
// <Code> of a Query API, or as the JSON "__type" or "code" of a JSON
// API, bare or after a namespace and "#".
func hasCode(text, code string) bool {
	return strings.Contains(text, "<Code>"+code+"</Code>") ||
		strings.Contains(text, `"`+code+`"`) ||
		strings.Contains(text, "#"+code+`"`)
}

// expiredError is the permanent error of an expired session token.
type expiredError struct {
	provider string
	static   bool
	err      error
}

func (e *expiredError) Error() string {
	hint := "new credentials were refused too"
	if e.static {
		hint = "kwatch cannot refresh a configured sessionToken; use " +
			"IAM roles for service accounts or EKS Pod Identity"
	}
	return fmt.Sprintf("%s: %v (%s): %s: %v", e.provider,
		ErrSessionTokenExpired, codeExpired, hint, e.err)
}

func (e *expiredError) Unwrap() []error {
	return []error{ErrSessionTokenExpired, e.err}
}
