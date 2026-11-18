package issues

import (
	"context"

	"github.com/abahmed/kwatch/internal/notification"
)

// IncidentCreator is optional. A tracker that implements it is given the
// message and the marker when it files the issue, so it can store the
// marker in a field of its own (a correlation id) and pick fields from
// the message, such as an urgency that follows the tier.
type IncidentCreator interface {
	CreateIncident(
		ctx context.Context, msg notification.Message,
		title, body, marker string,
	) (string, error)
}

// createIssue files the issue through the richest create the tracker
// offers.
func createIssue(
	ctx context.Context, tracker Tracker, msg notification.Message,
	title, body, marker string,
) (string, error) {
	if creator, ok := tracker.(IncidentCreator); ok {
		return creator.CreateIncident(ctx, msg, title, body, marker)
	}
	return tracker.Create(ctx, title, body)
}
