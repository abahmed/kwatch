package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fileBytes(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Size()
}

// fill writes n large state values and deletes all but "keep".
func fillAndDelete(t *testing.T, path string, n int) {
	t.Helper()
	s := openStoreAt(t, path)
	claim(t, s)
	values := state(s)
	big := strings.Repeat("x", 4096)
	for i := 0; i < n; i++ {
		require.NoError(t, values.Put(strings.Repeat("k", i+1), big))
	}
	for i := 0; i < n; i++ {
		require.NoError(t, values.Delete(strings.Repeat("k", i+1)))
	}
	require.NoError(t, values.Put("keep", "v"))
	require.NoError(t, s.Close())
}

func TestOpenRewritesFileOverCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	fillAndDelete(t, path, 200)
	before := fileBytes(t, path)

	s, err := Open(path, Options{
		Now: newFakeClock().Now, SizeCap: 64 << 10,
		FreeSpace: func(string) (uint64, error) { return 1 << 40, nil },
	})
	require.NoError(t, err)
	defer s.Close()

	assert.Less(t, fileBytes(t, path), before)
	got, found, err := state(s).Get("keep")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "v", got)
	assert.Equal(t, uint64(2), claim(t, s), "the epoch survives")
	_, err = os.Stat(path + ".compact")
	assert.True(t, os.IsNotExist(err))
}

func TestOpenKeepsFileUnderCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	fillAndDelete(t, path, 200)
	before := fileBytes(t, path)

	s := openStoreAt(t, path)
	defer s.Close()

	assert.Equal(t, before, fileBytes(t, path))
}

func TestOpenSkipsRewriteWithoutFreeSpace(t *testing.T) {
	for name, free := range map[string]freeSpaceFunc{
		"too little": func(string) (uint64, error) { return 1 << 10, nil },
		"unknown": func(string) (uint64, error) {
			return 0, errors.New("statfs failed")
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			fillAndDelete(t, path, 200)
			before := fileBytes(t, path)

			s, err := Open(path, Options{
				Now: newFakeClock().Now, SizeCap: 64 << 10,
				FreeSpace: free,
			})
			require.NoError(t, err)
			defer s.Close()

			assert.Equal(t, before, fileBytes(t, path))
			_, found, err := state(s).Get("keep")
			require.NoError(t, err)
			assert.True(t, found)
			_, err = os.Stat(path + ".compact")
			assert.True(t, os.IsNotExist(err))
		})
	}
}
