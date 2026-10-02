package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSourceAccessCoversEveryRegistration(t *testing.T) {
	listed := map[Resource]map[string]bool{}
	required := map[Resource]bool{}
	for _, access := range SourceAccess() {
		if listed[access.Resource] == nil {
			listed[access.Resource] = map[string]bool{}
		}
		listed[access.Resource][access.Verb] = true
		if access.Required {
			required[access.Resource] = true
		}
	}
	for _, r := range registrations() {
		assert.True(t, listed[r.resource]["list"], r.resource.Name)
		assert.True(t, listed[r.resource]["watch"], r.resource.Name)
		assert.Equal(t, r.required(), required[r.resource], r.resource.Name)
		assert.NotEmpty(t, r.resource.Name)
	}
	assert.True(t, listed[eventsResource]["watch"])
}

func TestSourceAccessRequiresOnlyPodsAndNodes(t *testing.T) {
	required := map[string]bool{}
	for _, access := range SourceAccess() {
		if access.Required {
			required[access.Resource.Name] = true
		}
	}
	assert.Equal(t, map[string]bool{"pods": true, "nodes": true}, required)
}

func TestSourceAccessEnrichmentIsOptional(t *testing.T) {
	for _, access := range SourceAccess() {
		switch access.Resource.Name {
		case "nodes/stats", "nodes/metrics", "pods/log", "apiservices",
			"leases":
			assert.False(t, access.Required, access.Resource.Name)
		}
	}
}

func TestSourceAccessGetsNodesButNotPods(t *testing.T) {
	gets := map[string]bool{}
	for _, access := range SourceAccess() {
		if access.Verb == "get" {
			gets[access.Resource.Name] = true
		}
	}
	assert.True(t, gets["nodes"])
	assert.False(t, gets["pods"], "pod get is namespaced, see InstallAccess")
}

func TestInstallAccessScopesPodGetToOwnNamespace(t *testing.T) {
	assert.Equal(t, []Access{{Resource: Resource{Name: "pods"},
		Verb: "get", Namespace: "kwatch"}}, InstallAccess("kwatch"))
}

func TestSourceAccessReadsKubeletDirectly(t *testing.T) {
	gets := map[string]bool{}
	for _, access := range SourceAccess() {
		if access.Verb == "get" {
			gets[access.Resource.Name] = true
		}
	}
	assert.True(t, gets["nodes/stats"])
	assert.True(t, gets["nodes/metrics"])
	assert.False(t, gets["nodes/proxy"], "the API server proxy is not used")
}

func TestWithoutResourceDropsSecretChecks(t *testing.T) {
	checks := WithoutResource(SourceAccess(), SecretsResource)
	for _, check := range checks {
		assert.NotEqual(t, SecretsResource, check.Resource)
	}
	assert.Len(t, checks, len(SourceAccess())-2, "list and watch dropped")
}
