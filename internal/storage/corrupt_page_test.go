package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedPages writes enough values to fill many pages and returns the
// page size and page count of the closed file.
func seedPages(t *testing.T, path string) (int64, int64) {
	t.Helper()
	s := openStoreAt(t, path)
	claim(t, s)
	values := map[string]Item[string]{}
	for i := 0; i < 2000; i++ {
		values[fmt.Sprintf("key-%04d", i)] = Item[string]{
			Value: strings.Repeat("v", 200),
		}
	}
	require.NoError(t, state(s).PutAll(values))
	pageSize := int64(s.db.Info().PageSize)
	require.NoError(t, s.Close())
	return pageSize, fileBytes(t, path) / pageSize
}

// damagePage overwrites one whole page with garbage, as a torn write or
// a bad disk sector would.
func damagePage(t *testing.T, path string, pageSize, page int64) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	require.NoError(t, err)
	defer f.Close()
	garbage := []byte(strings.Repeat("\xff", int(pageSize)))
	_, err = f.WriteAt(garbage, page*pageSize)
	require.NoError(t, err)
}

// bbolt opens a file with a damaged data page and panics only when the
// page is read. Open reads every page, so the damage resets the file
// instead of crashing a later write.
func TestOpenResetsFileWithCorruptPage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	pageSize, pages := seedPages(t, path)
	require.Greater(t, pages, int64(8))
	damagePage(t, path, pageSize, pages/2)

	s := openStoreAt(t, path)
	defer s.Close()

	reset, ok := s.Reset()
	require.True(t, ok, "a corrupt page must reset the file")
	assert.Equal(t, ResetUnreadable, reset.Reason)
	claim(t, s)
	require.NoError(t, state(s).Put("k", "fresh"))
	_, found, err := state(s).Get("key-0001")
	require.NoError(t, err)
	assert.False(t, found, "the damaged history is gone")
}
