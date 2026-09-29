package startup

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/abahmed/kwatch/internal/model"
)

// memoryState is an in-memory StateStore for tests.
type memoryState struct {
	clusterID   string
	initialized bool
	version     string
	lastSeen    time.Time
	session     model.RuntimeSession
	announced   string
}

func (m *memoryState) EnsureClusterID(context.Context) (string, error) {
	if m.clusterID == "" {
		m.clusterID = uuid.New().String()
	}
	return m.clusterID, nil
}

func (m *memoryState) IsFirstRun(context.Context) (bool, error) {
	return !m.initialized, nil
}

func (m *memoryState) GetStoredVersion(context.Context) (string, error) {
	return m.version, nil
}

func (m *memoryState) MarkAsInitialized(
	_ context.Context, clusterID, version string,
) error {
	m.clusterID, m.version, m.initialized = clusterID, version, true
	return nil
}

func (m *memoryState) GetLastSeen(context.Context) (time.Time, error) {
	return m.lastSeen, nil
}

func (m *memoryState) SetLastSeen(_ context.Context, at time.Time) error {
	m.lastSeen = at
	return nil
}

func (m *memoryState) GetRuntimeSession(
	context.Context,
) (model.RuntimeSession, error) {
	return m.session, nil
}

func (m *memoryState) SaveRuntimeSession(
	_ context.Context, s model.RuntimeSession,
) error {
	m.session = s
	return nil
}

func (m *memoryState) ClaimStartupAnnouncement(
	_ context.Context, key string,
) (bool, error) {
	if m.announced == key {
		return false, nil
	}
	m.announced = key
	return true, nil
}
