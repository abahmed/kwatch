package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Two sessions on the same store must use the same digest key, or every
// Secret and ConfigMap would look changed after a restart.
func TestEnsureDigestKeyIsStableAcrossSessions(t *testing.T) {
	state := openTestStore(t)

	first, err := diskState{store: state}.EnsureDigestKey()
	require.NoError(t, err)
	second, err := diskState{store: state}.EnsureDigestKey()
	require.NoError(t, err)

	require.Len(t, first, digestKeySize)
	require.Equal(t, first, second)
}

func TestEnsureDigestKeyReplacesMalformedKey(t *testing.T) {
	state := openTestStore(t)
	disk := diskState{store: state}
	require.NoError(t, disk.put(stateDigestKey, []byte("short")))

	key, err := disk.EnsureDigestKey()

	require.NoError(t, err)
	require.Len(t, key, digestKeySize)
}

func TestSourceDigestKeyFallsBackWhenStoreIsClosed(t *testing.T) {
	state := openTestStore(t)
	require.NoError(t, state.Close())

	require.Nil(t, sourceDigestKey(state),
		"a nil key makes the source use a per-process key")
}
