package dashboard

import "github.com/abahmed/kwatch/internal/redact"

// maxDeliveries bounds the delivery trail the page shows: the newest.
const maxDeliveries = 16

// toDeliveries is the trail, oldest first, keeping the newest
// maxDeliveries, with channel names (also of the channel a fallback
// stood in for) and errors redacted again. cut counts the attempts
// left out.
func toDeliveries(trail []DeliveryAttempt) ([]DeliveryAttempt, int) {
	cut := 0
	if len(trail) > maxDeliveries {
		cut = len(trail) - maxDeliveries
		trail = trail[cut:]
	}
	if len(trail) == 0 {
		return nil, 0
	}
	out := make([]DeliveryAttempt, 0, len(trail))
	for _, a := range trail {
		if a.Channel != "" {
			a.Name = channelName(a.Channel)
		}
		if a.Fallback != "" {
			a.Fallback = channelName(a.Fallback)
		}
		a.Error = redact.Evidence(a.Error)
		a.Reason = redact.Evidence(a.Reason)
		out = append(out, a)
	}
	return out, cut
}
