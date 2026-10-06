package wecom

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestWecomOutputCannotCloseItsFence(t *testing.T) {
	c, rec := recorderWecom(t)
	m := providertest.Announce()
	m.Output = []string{"```", "[click](http://evil) <@zhangsan>"}
	require.NoError(t, c.SendIncident(context.Background(), m))

	content := markdownContent(t, rec)
	assert.Contains(t, content, "\n````\n```\n")
	assert.True(t, strings.HasSuffix(content, "\n````"))
	assert.NotContains(t, content, "<@")
}

func TestWecomLongOutputKeepsTheClosingFence(t *testing.T) {
	c, rec := recorderWecom(t)
	m := providertest.Announce()
	m.Output = []string{strings.Repeat("x", wecomTextLimit*2)}
	require.NoError(t, c.SendIncident(context.Background(), m))

	content := markdownContent(t, rec)
	assert.LessOrEqual(t, len(content), wecomTextLimit)
	assert.True(t, strings.HasSuffix(content, "\n```"))
}
