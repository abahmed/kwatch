package pipeline

import (
	"encoding/json"

	"github.com/abahmed/kwatch/internal/storage"
)

// causeChecksKey holds the cause self-check counts in the state bucket.
const causeChecksKey = "pipeline.causechecks"

var _ causeStore = (*diskIncidents)(nil)

// LoadCauseChecks implements causeStore.
func (d *diskIncidents) LoadCauseChecks() (json.RawMessage, bool, error) {
	return storage.StateValues[json.RawMessage](d.store).Get(causeChecksKey)
}

// SaveCauseChecks implements causeStore.
func (d *diskIncidents) SaveCauseChecks(raw json.RawMessage) error {
	values := storage.StateValues[json.RawMessage](d.store)
	return values.Put(causeChecksKey, raw)
}

// unexplainedKey holds the unexplained counts in the state bucket.
const unexplainedKey = "pipeline.unexplained"

var _ unexplainedStore = (*diskIncidents)(nil)

// LoadUnexplained implements unexplainedStore.
func (d *diskIncidents) LoadUnexplained() (json.RawMessage, bool, error) {
	return storage.StateValues[json.RawMessage](d.store).Get(unexplainedKey)
}

// SaveUnexplained implements unexplainedStore.
func (d *diskIncidents) SaveUnexplained(raw json.RawMessage) error {
	values := storage.StateValues[json.RawMessage](d.store)
	return values.Put(unexplainedKey, raw)
}
