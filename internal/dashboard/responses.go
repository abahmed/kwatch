package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/feature"
)

// IncidentListItem is one incident in the /api/incidents list.
type IncidentListItem struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Severity    string    `json:"severity"`
	State       string    `json:"state"`
	Namespace   string    `json:"namespace"`
	Workload    string    `json:"workload"`
	FirstSeen   time.Time `json:"firstSeen"`
	LastSeen    time.Time `json:"lastSeen"`
	RepeatCount int       `json:"repeatCount"`
	AckedBy     string    `json:"ackedBy,omitempty"`
	ResolvedAt  time.Time `json:"resolvedAt,omitempty"`
}

// IncidentDetail is one incident's full detail for /api/incidents/{id}.
type IncidentDetail struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Severity         string    `json:"severity"`
	State            string    `json:"state"`
	Namespace        string    `json:"namespace"`
	Workload         string    `json:"workload"`
	RootCause        string    `json:"rootCause"`
	SuggestedFix     string    `json:"suggestedFix,omitempty"`
	FirstSeen        time.Time `json:"firstSeen"`
	LastSeen         time.Time `json:"lastSeen"`
	RepeatCount      int       `json:"repeatCount"`
	ResolvedAt       time.Time `json:"resolvedAt,omitempty"`
	Timeline         []Event   `json:"timeline,omitempty"`
	KubernetesEvents []string  `json:"kubernetesEvents,omitempty"`
	CapturedLogLines []string  `json:"capturedLogLines,omitempty"`
	DeliveryAttempts []DeliveryAttempt `json:"deliveryAttempts,omitempty"`
	DocsLink         string    `json:"docsLink,omitempty"`
}

// Event is one timeline entry.
type Event struct {
	At     time.Time `json:"at"`
	Text   string    `json:"text"`
	Entity string    `json:"entity,omitempty"`
}

// DeliveryAttempt is one attempt to deliver the incident to a channel.
type DeliveryAttempt struct {
	Channel string    `json:"channel"`
	At      time.Time `json:"at"`
	Status  string    `json:"status"`
}

// StatusResponse is the /api/status response.
type StatusResponse struct {
	Version           string          `json:"version"`
	Edition           string          `json:"edition"`
	LastScan          time.Time       `json:"lastScan"`
	WatchedNamespaces []string        `json:"watchedNamespaces"`
	PerChannelHealth  []ChannelHealth `json:"perChannelHealth"`
}

// ChannelHealth is one alert channel's delivery health.
type ChannelHealth struct {
	Channel           string    `json:"channel"`
	LastSuccess       time.Time `json:"lastSuccess,omitempty"`
	LastError         time.Time `json:"lastError,omitempty"`
	LastErrorMessage  string    `json:"lastErrorMessage,omitempty"`
}

// editionString returns the human-readable edition name for API responses.
func editionString(e feature.Entitlements) string {
	if e == nil {
		return string(feature.CommunityEdition)
	}
	return string(e.Edition())
}
