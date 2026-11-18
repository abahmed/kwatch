package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateGitOpsLinks(t *testing.T) {
	cfg := DefaultConfig()
	cfg.App.GitOpsLinks = map[string]string{
		"argocd":  "https://gitops.example.com/applications/{app}",
		"flux":    "https://gitops.example.com/apps",
		"helm":    "ftp://gitops.example.com/{app}",
		"jenkins": "https://ci.example.com/{app}",
	}

	errs := errorTexts(validateGitOpsLinks(cfg))

	require.Len(t, errs, 3)
	assert.Contains(t, errs[0], `app.gitOpsLinks["flux"]`)
	assert.Contains(t, errs[1], `app.gitOpsLinks["helm"]`)
	assert.Contains(t, errs[2], `app.gitOpsLinks["jenkins"]`)
}

func TestGitOpsLinksReachRuntimePolicy(t *testing.T) {
	cfg := DefaultConfig()
	link := "https://gitops.example.com/applications/{app}"
	cfg.App.GitOpsLinks = map[string]string{"argocd": link}

	links := CompileRuntimeConfig(cfg).Policy().GitOpsLinks()
	links["argocd"] = "changed"

	assert.Equal(t, link,
		CompileRuntimeConfig(cfg).Policy().GitOpsLinks()["argocd"])
	assert.Empty(t, CompileRuntimeConfig(DefaultConfig()).Policy().
		GitOpsLinks())
}
