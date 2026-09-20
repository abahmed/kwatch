package change

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Tooling rewrites its own annotations on every reconcile; kopf's holds the
// whole object as JSON. None of that is something a person changed.
func TestDiffIgnoresToolingAnnotationChurn(t *testing.T) {
	before := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: "web", Namespace: "ns",
		Annotations: map[string]string{
			"kopf.zalando.org/last-handled-configuration": `{"spec":{"a":1}}`,
			"deployment.kubernetes.io/revision":           "3",
			"team":                                        "payments",
		},
	}}
	after := before.DeepCopy()
	after.Annotations["kopf.zalando.org/last-handled-configuration"] =
		`{"spec":{"a":2}}`
	after.Annotations["deployment.kubernetes.io/revision"] = "4"

	result := Diff(before, after)
	assert.Empty(t, result.Fields)
	assert.Equal(t, result.BeforeHash, result.AfterHash)

	after.Annotations["team"] = "platform"
	result = Diff(before, after)
	if assert.Len(t, result.Fields, 1) {
		assert.Equal(t, "metadata.annotations.team", result.Fields[0].Path)
	}
}
