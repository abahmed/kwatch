package feishu

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// Feishu limits the whole request body to 20 KB. JSON escapes "<" as six
// bytes and a CJK rune takes three, so a text under the rune limit can
// still overflow the body.
func TestFeiShuBodyStaysUnderTheLimit(t *testing.T) {
	for name, filler := range map[string]string{
		"escaped": "<", "multibyte": "漢", "plain": "x",
	} {
		t.Run(name, func(t *testing.T) {
			c, rec := recorderFeiShu(t)
			m := providertest.Announce()
			m.Note = "🔴 " + strings.Repeat(filler, 40000)
			require.NoError(t, c.SendIncident(context.Background(), m))
			assertBodyFits(t, rec)
			require.NoError(t, c.SendMessage(context.Background(),
				strings.Repeat(filler, 40000)))
			assertBodyFits(t, rec)
		})
	}
}

func assertBodyFits(t *testing.T, rec *providertest.Recorder) {
	t.Helper()
	body := rec.Last(t).Body
	assert.LessOrEqual(t, len(body), feiShuBodyLimit)
	assert.True(t, json.Valid(body), "truncated body must stay JSON")
}
