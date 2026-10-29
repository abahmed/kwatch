package discord

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestEmbedDescriptionFenceSurvivesTruncation(t *testing.T) {
	m := providertest.Announce()
	m.Output = []string{"```@everyone", strings.Repeat("x", maxEmbedTotal)}
	got := description(m, maxEmbedDescription)
	assert.LessOrEqual(t, len(got), maxEmbedDescription)
	assert.True(t, strings.HasSuffix(got, "\n````"), got[len(got)-20:])
	assert.NotContains(t, got, "@everyone")
}
