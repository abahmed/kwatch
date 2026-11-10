package github

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestGithubRejectsInvalidURL(t *testing.T) {
	rec := providertest.NewRecorder(t)
	build := func(endpoint string) *Github {
		return NewGithub(map[string]interface{}{
			"url":   endpoint,
			"token": "t",
			"owner": "o",
			"repo":  "r",
		}, "dev", rec.Dependencies())
	}
	for _, endpoint := range []string{"not a url", "ftp://x", "/relative"} {
		assert.Nil(t, build(endpoint), endpoint)
	}
	assert.NotNil(t, build(rec.URL()))
	assert.NotNil(t, build(""), "unset uses the default")
}

// A server URL written with a trailing slash still builds single-slash
// API paths.
func TestGithubTrimsTrailingSlash(t *testing.T) {
	rec := providertest.NewRecorder(t)
	g := NewGithub(map[string]interface{}{
		"url": rec.URL() + "/api/v3/", "token": "t", "owner": "o",
		"repo": "r",
	}, "dev", rec.Dependencies())
	assert.Equal(t, rec.URL()+"/api/v3/repos/o/r/issues", g.url)
	assert.Equal(t, rec.URL()+"/api/v3/search/issues", g.searchURL)
}
