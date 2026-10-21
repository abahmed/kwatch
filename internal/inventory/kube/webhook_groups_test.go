package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func hookRule(groups ...string) admissionv1.RuleWithOperations {
	return admissionv1.RuleWithOperations{
		Rule: admissionv1.Rule{APIGroups: groups,
			Resources: []string{"*"}}}
}

// A webhook configuration says which custom API groups it admits;
// wildcards and built-in groups say nothing about a controller.
func TestWebhookConfigurationRecordsTheCustomGroupsItCovers(t *testing.T) {
	config := &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "v"},
		Webhooks: []admissionv1.ValidatingWebhook{
			{Name: "a", Rules: []admissionv1.RuleWithOperations{
				hookRule("widgets.example.com", "apps", "*", ""),
			}},
			{Name: "b", Rules: []admissionv1.RuleWithOperations{
				hookRule("gadgets.example.com", "widgets.example.com"),
			}},
		},
	}

	d, ok := kube.WebhookSchema{}.Describe(config)

	require.True(t, ok)
	assert.Equal(t, "gadgets.example.com widgets.example.com",
		text(d, kube.AttrWebhookGroups))
}

func TestWebhookConfigurationOfBuiltinGroupsHasNoGroups(t *testing.T) {
	config := &admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "m"},
		Webhooks: []admissionv1.MutatingWebhook{{Name: "a",
			Rules: []admissionv1.RuleWithOperations{hookRule("", "apps")}}},
	}

	d, ok := kube.WebhookSchema{Mutating: true}.Describe(config)

	require.True(t, ok)
	assert.NotContains(t, d.Attributes, kube.AttrWebhookGroups)
}
