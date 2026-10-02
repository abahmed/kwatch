package api

import (
	"context"

	"github.com/abahmed/kwatch/internal/notification"
)

// IncidentProvider renders one incident message in the provider's own
// format. Chat providers send the narrative Note, paging providers open
// and resolve one alert per Message.Key, issue trackers keep one issue per
// key, and SMS or push providers send the Short lead.
type IncidentProvider interface {
	SendIncident(context.Context, notification.Message) error
}
