package kube

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestPodRecordsItsPullSecrets(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{ImagePullSecrets: []corev1.
		LocalObjectReference{{Name: "regcred"}, {Name: "backup"}}}}
	attrs := map[string]inventory.Value{}
	setPullSecrets(attrs, pod)
	assert.Equal(t, "regcred,backup", attrs[AttrPullSecrets].AsText())
	assert.Equal(t, []string{"regcred", "backup"},
		PullSecretNames("regcred,backup"))

	none := map[string]inventory.Value{}
	setPullSecrets(none, &corev1.Pod{})
	assert.Empty(t, none, "no pull secret leaves no attribute")
	assert.Empty(t, PullSecretNames(""))
}

func TestLoginSecretRecordsWhenItLastChanged(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	rotated := created.AddDate(0, 1, 0)
	secret := &corev1.Secret{
		Type: corev1.SecretTypeDockerConfigJson,
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.NewTime(created),
		},
	}
	attrs := map[string]inventory.Value{}
	setLoginChanged(attrs, secret)
	assert.True(t, created.Equal(attrs[AttrChanged].AsTime()), "creation")

	secret.ManagedFields = []metav1.ManagedFieldsEntry{{
		Manager: "kubectl", Operation: metav1.ManagedFieldsOperationUpdate,
		Time: &metav1.Time{Time: rotated},
	}}
	setLoginChanged(attrs, secret)
	assert.True(t, rotated.Equal(attrs[AttrChanged].AsTime()), "last write")
}

func TestOtherSecretsCarryNoChangedTime(t *testing.T) {
	secret := &corev1.Secret{
		Type: corev1.SecretTypeOpaque,
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
	}
	attrs := map[string]inventory.Value{}
	setLoginChanged(attrs, secret)
	assert.Empty(t, attrs)
}

func TestPullRefusalIsReadBeforeTheMessageIsCut(t *testing.T) {
	tail := "https://reg.example/token?" + strings.Repeat("scope=x&", 80) +
		": 401 Unauthorized: unauthorized: authentication required"
	tests := []struct{ message, want string }{
		{"failed to authorize: " + tail, "unauthorized: authentication required"},
		{"unexpected status: 403 Forbidden", "403 Forbidden"},
		{"pull access denied, repository does not exist",
			"pull access denied, repository does not exist"},
		{"Error response: no basic auth credentials",
			"no basic auth credentials"},
		{"toomanyrequests: You have reached your pull rate limit", ""},
		{"manifest unknown: manifest unknown", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, PullRefusal(tt.message), tt.message[:20])
	}
}
