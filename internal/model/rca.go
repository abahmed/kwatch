package model

import "time"

// RCARecord is the persisted, bounded learning record used to improve
// diagnosis confidence across incident lifecycles. It belongs to model
// because both insight and persistence use the wire format.
type RCARecord struct {
	Fingerprint     string    `json:"fingerprint"`
	CauseClass      string    `json:"causeClass"`
	Observations    int       `json:"observations"`
	Resolved        int       `json:"resolved"`
	Recurred        int       `json:"recurred"`
	UnknownOutcomes int       `json:"unknownOutcomes"`
	ConfidenceBias  float64   `json:"confidenceBias"`
	LastOutcome     string    `json:"lastOutcome"`
	LastSeen        time.Time `json:"lastSeen"`
}
