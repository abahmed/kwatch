package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSourceAccessCoversEveryRegistration(t *testing.T) {
	required := map[Resource]map[string]bool{}
	for _, access := range SourceAccess() {
		if access.Required {
			if required[access.Resource] == nil {
				required[access.Resource] = map[string]bool{}
			}
			required[access.Resource][access.Verb] = true
		}
	}
	for _, r := range registrations() {
		assert.True(t, required[r.resource]["list"], r.resource.Name)
		assert.True(t, required[r.resource]["watch"], r.resource.Name)
		assert.NotEmpty(t, r.resource.Name)
	}
	assert.True(t, required[Resource{Name: "events"}]["watch"])
}

func TestSourceAccessEnrichmentIsOptional(t *testing.T) {
	for _, access := range SourceAccess() {
		switch access.Resource.Name {
		case "nodes/proxy", "pods/log", "apiservices", "leases":
			assert.False(t, access.Required, access.Resource.Name)
		}
	}
}
