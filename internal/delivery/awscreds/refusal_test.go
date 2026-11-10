package awscreds

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/signing"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// countingSource is a refreshable source that counts Invalidate.
type countingSource struct{ invalidated int }

func (c *countingSource) Credentials(
	context.Context,
) (signing.Credentials, error) {
	return signing.Credentials{AccessKeyID: "ASIA"}, nil
}

func (c *countingSource) Invalidate() { c.invalidated++ }

func awsRefusal(code string) (error, []byte) {
	body := "<ErrorResponse><Error><Type>Sender</Type><Code>" + code +
		"</Code><Message>The security token included in the request " +
		"is expired</Message></Error></ErrorResponse>"
	return transport.Permanent(&transport.StatusError{Provider: "SNS",
		StatusCode: 403,
		Body: "[response body omitted because it may contain " +
			"credentials]"}), []byte(body)
}

func TestRefusalsRetryOnceForRefreshableSources(t *testing.T) {
	for _, code := range []string{codeExpired, codeInvalidToken,
		codeSignatureFail} {
		t.Run(code, func(t *testing.T) {
			src := &countingSource{}
			var r Refusals
			err, answer := awsRefusal(code)
			got := r.Check("SNS", src, err, answer)
			if transport.IsPermanent(got) || src.invalidated != 1 ||
				!errors.Is(got, ErrCredentialsRefused) {
				t.Fatalf("first refusal: %v, invalidated %d", got,
					src.invalidated)
			}
			if _, ok := transport.StatusOf(got); ok {
				t.Fatal("the retry still carries the 403")
			}
			got = r.Check("SNS", src, err, answer)
			if !transport.IsPermanent(got) || src.invalidated != 1 {
				t.Fatalf("second refusal: %v, invalidated %d", got,
					src.invalidated)
			}
			if r.Check("SNS", src, nil, nil) != nil {
				t.Fatal("success returned an error")
			}
			if transport.IsPermanent(r.Check("SNS", src, err, answer)) {
				t.Fatal("a success did not reset the retry")
			}
		})
	}
}

func TestRefusalsStaticExpiredTokenIsPermanentAndWorded(t *testing.T) {
	var r Refusals
	err, answer := awsRefusal(codeExpired)
	got := r.Check("SES", Static{}, err, answer)
	if !transport.IsPermanent(got) ||
		!errors.Is(got, ErrSessionTokenExpired) {
		t.Fatalf("err = %v", got)
	}
	if code, _ := transport.StatusOf(got); code != 403 {
		t.Fatalf("status = %d, want 403 kept", code)
	}
	if !strings.Contains(got.Error(), "AWS session token expired") ||
		!strings.Contains(got.Error(), "EKS Pod Identity") {
		t.Fatalf("message = %q", got.Error())
	}
}

func TestRefusalsLeaveOtherFailuresAlone(t *testing.T) {
	var r Refusals
	src := &countingSource{}
	err, answer := awsRefusal("AuthorizationError")
	if got := r.Check("SNS", src, err, answer); got != err {
		t.Fatalf("err = %v", got)
	}
	err, answer = awsRefusal(codeSignatureFail)
	if got := r.Check("SNS", Static{}, err, answer); got != err {
		t.Fatalf("static signature mismatch changed: %v", got)
	}
	server := errors.New("call to SNS returned status code 500")
	if got := r.Check("SNS", src, server, []byte("<Code>ExpiredToken"+
		"</Code>")); got != server || src.invalidated != 0 {
		t.Fatalf("non-403 changed: %v", got)
	}
}

// The JSON APIs' names for the same refusals mean the same, in either
// answer format.
func TestRefusalsKnowBothAWSNamingStyles(t *testing.T) {
	for _, c := range []struct{ sent, want, body string }{
		{"ExpiredTokenException", codeExpired, ""},
		{"UnrecognizedClientException", codeInvalidToken, ""},
		{"ExpiredTokenException", codeExpired,
			`{"__type":"com.amazon.coral.service#ExpiredTokenException",` +
				`"message":"The security token is expired"}`},
		{"UnrecognizedClientException", codeInvalidToken,
			`{"__type":"UnrecognizedClientException","message":"no"}`},
	} {
		err, answer := awsRefusal(c.sent)
		if c.body != "" {
			answer = []byte(c.body)
		}
		if got := refusalCode(err, answer); got != c.want {
			t.Errorf("%s in %q: code %q, want %q", c.sent, answer, got,
				c.want)
		}
	}
	var r Refusals
	err, answer := awsRefusal("ExpiredTokenException")
	if got := r.Check("SES", Static{}, err, answer); !errors.Is(got,
		ErrSessionTokenExpired) || !transport.IsPermanent(got) {
		t.Fatalf("static ExpiredTokenException: %v", got)
	}
}
