package teams

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

// A legacy Office 365 connector webhook answers HTTP 200 even when it
// did not deliver the message: the body then reads "Webhook message
// delivery failed with error: Microsoft Teams endpoint returned HTTP
// error 429 ...". A delivered message is answered with "1". Taking
// that 200 for success would drop the alert without a retry or the
// fallback.
var connectorError = regexp.MustCompile(`returned HTTP error (\d{3})`)

// statusTooManyRequests is HTTP 429.
const statusTooManyRequests = 429

// connectorErrorLimit bounds how much of the answer an error quotes.
const connectorErrorLimit = 200

// connectorFailure reads the body of a 2xx answer and returns the
// failure it reports, classified by the status it names like a real
// HTTP status: 429 is a rate limit, another client error is permanent,
// anything else is retried.
func connectorFailure(body []byte) error {
	match := connectorError.FindSubmatch(body)
	if match == nil {
		return nil
	}
	status, _ := strconv.Atoi(string(match[1]))
	if status == statusTooManyRequests {
		return &ratelimit.Error{Provider: "Teams", StatusCode: status}
	}
	err := fmt.Errorf("teams connector did not deliver the message: %s",
		notification.Truncate(string(body), connectorErrorLimit))
	if transport.IsPermanentHTTPStatus(status) {
		return transport.Permanent(err)
	}
	return err
}
