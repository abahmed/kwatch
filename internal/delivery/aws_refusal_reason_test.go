package delivery

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/awscreds"
	"github.com/abahmed/kwatch/internal/delivery/signing"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// rotatingSource is a refreshable AWS credential source.
type rotatingSource struct{}

func (rotatingSource) Credentials(
	context.Context,
) (signing.Credentials, error) {
	return signing.Credentials{AccessKeyID: "ASIA"}, nil
}

func (rotatingSource) Invalidate() {}

func expiredTokenAnswer() []byte {
	return []byte("<ErrorResponse><Error><Code>ExpiredToken</Code>" +
		"</Error></ErrorResponse>")
}

// An expired static session token is still a rejection, but the trail
// says why instead of "access denied (403)".
func TestFailureReasonNamesAnExpiredAWSSessionToken(t *testing.T) {
	var refusals awscreds.Refusals
	err := refusals.Check("SNS", awscreds.Static{},
		statusErr(403, ""), expiredTokenAnswer())
	if got := failureReason(err); got != "AWS session token expired" {
		t.Fatalf("reason = %q", got)
	}
	var pacer sendPacer
	if !pacer.rejectionMarksProvider("SNS", err) {
		t.Fatal("an expired static token did not mark the provider")
	}
}

// The first refusal of refreshable credentials is retried with new
// ones; it must not mark the provider rejecting.
func TestFirstAWSRefusalOfRefreshableCredentialsIsRetried(t *testing.T) {
	var refusals awscreds.Refusals
	err := refusals.Check("SNS", rotatingSource{},
		statusErr(403, ""), expiredTokenAnswer())
	if got := failureReason(err); got !=
		"AWS credentials refused, fetching new ones" {
		t.Fatalf("reason = %q", got)
	}
	if transport.IsPermanent(err) {
		t.Fatal("the first refusal is permanent, so it would mark " +
			"the provider rejecting")
	}
}
