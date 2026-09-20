package insight

import (
	"sort"
	"strings"
	"sync"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/model"
)

// RCARecord remains an insight alias for callers that work with diagnosis,
// while the persisted wire type is owned by the shared model package.
type RCARecord = model.RCARecord

type FeedbackStore struct {
	mu      sync.RWMutex
	records map[string]RCARecord
	now     clock.Clock
	// active remembers the diagnosis attached to a live incident. Resolve
	// notifications do not carry the original Insight, so using only the
	// reason would incorrectly update every learned pattern for that reason.
	active map[string]string
}

const maxFeedbackRecords = 500

// NewFeedbackStoreWithClock fixes the time source at construction so feedback
// timestamps cannot change underneath concurrent observations.
func NewFeedbackStoreWithClock(now clock.Clock) *FeedbackStore {
	now = clock.Require(now)
	return &FeedbackStore{
		records: make(map[string]RCARecord),
		active:  make(map[string]string),
		now:     now,
	}
}

func (s *FeedbackStore) Observe(inc *model.Incident, action model.IncidentAction, pattern string) {
	if s == nil || inc == nil || pattern == "" {
		return
	}
	key := feedbackKey(inc, pattern)
	s.mu.Lock()
	now := s.now
	incidentKey := ""
	if inc != nil {
		incidentKey = string(inc.Key)
	}
	if action == model.ActionResolved {
		if learned := s.active[incidentKey]; learned != "" {
			key = feedbackKey(inc, learned)
			pattern = learned
		}
		delete(s.active, incidentKey)
	} else if incidentKey != "" && pattern != "" {
		s.active[incidentKey] = pattern
	}
	record := s.records[key]
	record.Fingerprint, record.CauseClass = key, pattern
	record.LastSeen = now.Now()
	if action == model.ActionResolved {
		record.Resolved++
		record.LastOutcome = "resolved"
		if record.Observations >= 3 {
			record.ConfidenceBias += 0.01
		}
	} else if action == model.ActionCreate {
		record.Observations++
		if record.LastOutcome == "resolved" {
			record.Recurred++
			record.ConfidenceBias -= 0.03
		}
		record.LastOutcome = "observed"
	}
	if record.ConfidenceBias > 0.15 {
		record.ConfidenceBias = 0.15
	}
	if record.ConfidenceBias < -0.15 {
		record.ConfidenceBias = -0.15
	}
	s.records[key] = record
	s.prune()
	s.mu.Unlock()
}

func (s *FeedbackStore) prune() {
	for len(s.records) > maxFeedbackRecords {
		oldestKey := ""
		for candidate, item := range s.records {
			if oldestKey == "" || s.records[oldestKey].LastSeen.After(item.LastSeen) {
				oldestKey = candidate
			}
		}
		if oldestKey == "" {
			return
		}
		delete(s.records, oldestKey)
	}
}

func (s *FeedbackStore) Bias(key string) float64 {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record := s.records[key]
	if record.Observations < 3 {
		return 0
	}
	return record.ConfidenceBias
}

func (s *FeedbackStore) Snapshot() []RCARecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RCARecord, 0, len(s.records))
	for _, record := range s.records {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].LastSeen.Equal(out[j].LastSeen) {
			return out[i].LastSeen.Before(out[j].LastSeen)
		}
		return out[i].Fingerprint < out[j].Fingerprint
	})
	return out
}

func (s *FeedbackStore) Restore(records []RCARecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = make(map[string]RCARecord, len(records))
	s.active = make(map[string]string)
	for _, record := range records {
		if record.Fingerprint != "" {
			s.records[record.Fingerprint] = record
		}
	}
}

// feedbackKey intentionally excludes pod/workload identity. A replacement
// object should contribute to the same learned outcome when it exhibits the
// same failure pattern, while the bounded store prevents unbounded growth.
func feedbackKey(inc *model.Incident, pattern string) string {
	reason := "unknown"
	if inc != nil && inc.Reason != "" {
		reason = strings.ToLower(strings.TrimSpace(inc.Reason))
	}
	return reason + "|" + strings.ToLower(strings.TrimSpace(pattern))
}
