package upgrader

import (
	"context"
)

// memoryVersions is an in-memory VersionTracker for tests.
type memoryVersions struct{ version string }

func (m *memoryVersions) GetNotifiedVersion(context.Context) string {
	return m.version
}

func (m *memoryVersions) SetNotifiedVersion(
	_ context.Context, version string,
) error {
	m.version = version
	return nil
}
