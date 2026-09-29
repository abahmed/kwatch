package delivery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultMaxBytes(t *testing.T) {
	assert.Equal(t, 2000, defaultMaxBytes("discord"))
	assert.Equal(t, 4096, defaultMaxBytes("telegram"))
	assert.Equal(t, 28000, defaultMaxBytes("teams"))
	assert.Equal(t, 40000, defaultMaxBytes("slack"))
	assert.Equal(t, 0, defaultMaxBytes("webhook"))
}

func TestProviderCatalogIdentitySelectsPayloadPolicy(t *testing.T) {
	entry := providerEntry{
		catalogName: "teams",
		provider: &recordingProvider{
			name: "Microsoft Teams", messages: make(chan string, 1),
		},
	}

	assert.Equal(t, "teams", entry.lookupName())
	assert.Equal(t, 28000, defaultMaxBytes(entry.lookupName()))
}

func TestTruncateMsgRejectsNonPositiveBudget(t *testing.T) {
	assert.Empty(t, truncateMsg("sensitive payload", 0))
	assert.Empty(t, truncateMsg("sensitive payload", -1))
}

func TestProviderPayloadPolicyIsExplicit(t *testing.T) {
	for name, policy := range payloadPolicies {
		assert.Equal(t, payloadBounded, policy.mode, name)
		assert.Positive(t, policy.maxBytes, name)
	}
	assert.Equal(t, payloadProviderOwn, providerPayloadPolicy("webhook").mode)
	assert.Zero(t, providerPayloadPolicy("webhook").maxBytes)
}

func TestCompileTemplates(t *testing.T) {
	am := *newTestManager()
	setTestTemplates(&am, map[string]string{
		"crashloopbackoff": "ALERT {{.Incident.Name}} — {{.Action}}",
	})
	if am.templates == nil {
		t.Fatal("templates map is nil")
	}
	if _, ok := am.templates["crashloopbackoff"]; !ok {
		t.Fatal("crashloopbackoff template not found")
	}
}

func TestCompileTemplatesEmpty(t *testing.T) {
	am := *newTestManager()
	setTestTemplates(&am, nil)
	if am.templates != nil {
		t.Fatal("expected nil templates")
	}
	setTestTemplates(&am, map[string]string{})
	if am.templates != nil {
		t.Fatal("expected nil templates for empty map")
	}
}
