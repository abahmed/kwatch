package discord

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestIncidentContentFenceSurvivesTruncation(t *testing.T) {
	m := providertest.Announce()
	m.Output = []string{"```@everyone", strings.Repeat("x", maxContent)}
	content := incidentContent(m)
	assert.LessOrEqual(t, len(content), maxContent)
	assert.True(t, strings.HasSuffix(content, "\n````"), content[len(content)-20:])
	assert.NotContains(t, content, "@everyone")
}
