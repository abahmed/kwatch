package pipeline

import (
	"encoding/json"
	"errors"
	"sync"
)

var errCountsStore = errors.New("counts store down")

// countsStore is a HistoryStore that also keeps the cause self-check and
// unexplained counts, and fails their writes while err is set.
type countsStore struct {
	failingHistory
	cmu sync.Mutex
	err error
	// checks and unexplained are the last values written; the saves
	// slices hold every attempt, failed or not.
	checks, unexplained json.RawMessage
	checkSaves          []string
	unexplainedSaves    []string
}

func (s *countsStore) setErr(err error) {
	s.cmu.Lock()
	s.err = err
	s.cmu.Unlock()
}

func (s *countsStore) LoadCauseChecks() (json.RawMessage, bool, error) {
	s.cmu.Lock()
	defer s.cmu.Unlock()
	return s.checks, s.checks != nil, nil
}

func (s *countsStore) SaveCauseChecks(raw json.RawMessage) error {
	s.cmu.Lock()
	defer s.cmu.Unlock()
	s.checkSaves = append(s.checkSaves, string(raw))
	if s.err != nil {
		return s.err
	}
	s.checks = raw
	return nil
}

func (s *countsStore) LoadUnexplained() (json.RawMessage, bool, error) {
	s.cmu.Lock()
	defer s.cmu.Unlock()
	return s.unexplained, s.unexplained != nil, nil
}

func (s *countsStore) SaveUnexplained(raw json.RawMessage) error {
	s.cmu.Lock()
	defer s.cmu.Unlock()
	s.unexplainedSaves = append(s.unexplainedSaves, string(raw))
	if s.err != nil {
		return s.err
	}
	s.unexplained = raw
	return nil
}
