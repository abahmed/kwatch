package enrichment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogsUnavailableRecognisesKubeletSentinel(t *testing.T) {
	assert.True(t, LogsUnavailable(
		"unable to retrieve container logs for containerd://cffc1f44",
	))
	assert.True(t, LogsUnavailable(
		"\nunable to retrieve container logs for docker://abc\n",
	))
	assert.False(t, LogsUnavailable("panic: runtime error"))
	assert.False(t, LogsUnavailable(""))
}
