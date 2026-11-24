package upgrader

import (
	"context"
	"errors"
)

// memoryVersions is an in-memory VersionTracker for tests.
type memoryVersions struct {
	version string
	// failSet makes every write fail and nothing is stored.
	failSet bool
}

func (m *memoryVersions) GetNotifiedVersion(context.Context) string {
	return m.version
}

func (m *memoryVersions) SetNotifiedVersion(
	_ context.Context, version string,
) error {
	if m.failSet {
		return errors.New("state write failed")
	}
	m.version = version
	return nil
}
