package matrix

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Matrix event is limited to 64 KiB in all; "<" costs six bytes in
// JSON, so both bodies are cut until the marshalled event fits.
func TestMatrixCutsOverlongEvent(t *testing.T) {
	for _, filler := range []string{"x", "<"} {
		c, rec := recorderMatrix(t)
		require.NoError(t, c.SendMessage(context.Background(),
			strings.Repeat(filler, 200000)))
		assert.Less(t, len(rec.Last(t).Body), 65536)
		got := rec.Last(t).JSON(t)
		assert.NotEmpty(t, got["body"])
		assert.NotEmpty(t, got["formatted_body"])
	}
}
