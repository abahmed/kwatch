package dashboard

import "time"

// Timing is how fast kwatch saw and told: StartedAt is when the first
// failure began (as Kubernetes reported it), NoticedAt when kwatch
// opened the incident, ToldAt when a channel first accepted a message
// about it and ToldVia that channel. Each is omitted when unknown.
type Timing struct {
	StartedAt time.Time `json:"startedAt,omitzero"`
	NoticedAt time.Time `json:"noticedAt,omitzero"`
	ToldAt    time.Time `json:"toldAt,omitzero"`
	ToldVia   string    `json:"toldVia,omitempty"`
}

// SameTimeItem is another issue that started within a few minutes of
// this one. Relation is "same node", "same namespace" or "cluster".
type SameTimeItem struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Severity  string    `json:"severity"`
	State     string    `json:"state"`
	Namespace string    `json:"namespace,omitempty"`
	StartedAt time.Time `json:"startedAt"`
	Relation  string    `json:"relation"`
}

// UsageHistory is one container's hourly peaks of one resource over
// the last 24 hours, oldest first. Metric is "memory" (bytes) or "cpu"
// (millicores); Limit is the container's limit in the same unit, 0
// when it has none.
type UsageHistory struct {
	Namespace string      `json:"namespace"`
	Pod       string      `json:"pod"`
	Container string      `json:"container"`
	Metric    string      `json:"metric"`
	Limit     int64       `json:"limit,omitempty"`
	Peaks     []UsagePeak `json:"peaks"`
}

// UsagePeak is the highest reading of one clock hour.
type UsagePeak struct {
	Start time.Time `json:"start"`
	Value int64     `json:"value"`
}
