package gitea

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestGiteaEscapesOwnerAndRepoSegments(t *testing.T) {
	deps := providertest.NewRecorder(t).Dependencies()
	g := NewGitea(map[string]interface{}{
		"token": "t", "owner": "my org", "repo": "a/b?c",
		"url": "https://gitea.corp/api/v1",
	}, "dev", deps)
	require.NotNil(t, g)
	assert.Equal(t,
		"https://gitea.corp/api/v1/repos/my%20org/a%2Fb%3Fc/issues", g.url)
}
