package storage

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOutboxKeepsKeyOrderAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s := openStoreAt(t, path)
	claim(t, s)
	items := map[string]Item[record]{}
	for i := 3; i >= 1; i-- {
		items[fmt.Sprintf("%020d", i)] = Item[record]{Value: record{Num: i}}
	}
	require.NoError(t, OutboxValues[record](s).PutAll(items))
	require.NoError(t, s.Close())

	reopened := openStoreAt(t, path)
	t.Cleanup(func() { _ = reopened.Close() })
	var order []int
	require.NoError(t, OutboxValues[record](reopened).Range("",
		func(_ string, value record) error {
			order = append(order, value.Num)
			return nil
		}))

	require.Equal(t, []int{1, 2, 3}, order)
}
