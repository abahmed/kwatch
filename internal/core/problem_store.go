package core

import (
	"github.com/abahmed/kwatch/internal/knowledge/store"
	"github.com/abahmed/kwatch/internal/problem"
)

// diskProblems stores problem records in the state file, one key per
// problem, and deletes records the manager no longer holds.
type diskProblems struct {
	store *store.Store
}

// NewProblemStore persists problems in s.
func NewProblemStore(s *store.Store) ProblemStore {
	return diskProblems{store: s}
}

// LoadProblems implements ProblemStore.
func (d diskProblems) LoadProblems() ([]problem.Record, error) {
	var out []problem.Record
	err := d.store.ForEach(store.Problems, "",
		func(_ string, decode func(any) error) error {
			var record problem.Record
			if err := decode(&record); err != nil {
				return err
			}
			out = append(out, record)
			return nil
		})
	return out, err
}

// SaveProblems implements ProblemStore in one transaction.
func (d diskProblems) SaveProblems(records []problem.Record) error {
	values := make(map[string]any, len(records))
	for _, record := range records {
		values[record.ID] = record
	}
	return d.store.ReplaceAll(store.Problems, values)
}

// LoadFingerprints implements ProblemStore.
func (d diskProblems) LoadFingerprints() (map[string]string, error) {
	out := map[string]string{}
	err := d.store.ForEach(store.Snapshot, "",
		func(key string, decode func(any) error) error {
			var value string
			if err := decode(&value); err != nil {
				return err
			}
			out[key] = value
			return nil
		})
	return out, err
}

// SaveFingerprints implements ProblemStore.
func (d diskProblems) SaveFingerprints(values map[string]any) error {
	return d.store.ReplaceAll(store.Snapshot, values)
}
