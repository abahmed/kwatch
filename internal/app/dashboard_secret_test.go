package app

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func emptyDashboardSecret(name string) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: "kwatch"}}
}

// Every start, and every replica, reads the same credentials: the first
// start writes them once and a restart keeps them.
func TestDashboardCredentialsSurviveRestarts(t *testing.T) {
	t.Setenv(dashboardSecretEnv, "kwatch-dashboard")
	t.Setenv("POD_NAMESPACE", "kwatch")
	client := fake.NewSimpleClientset(emptyDashboardSecret("kwatch-dashboard"))
	ctx := context.Background()
	token, password := dashboardCredentials(ctx, client, "kwatch")
	assert.Len(t, token, 32)
	assert.Regexp(t,
		regexp.MustCompile(`^([A-HJ-NP-Z2-9]{4}-){3}[A-HJ-NP-Z2-9]{4}$`),
		password)
	for range 3 {
		again, pw := dashboardCredentials(ctx, client, "kwatch")
		assert.Equal(t, token, again)
		assert.Equal(t, password, pw)
	}
}

func TestDashboardCredentialsKeepValuesAnOperatorSet(t *testing.T) {
	t.Setenv(dashboardSecretEnv, "kwatch-dashboard")
	secret := emptyDashboardSecret("kwatch-dashboard")
	secret.Data = map[string][]byte{"password": []byte("MINE-MINE")}
	client := fake.NewSimpleClientset(secret)
	token, password := dashboardCredentials(
		context.Background(), client, "kwatch")
	assert.Equal(t, "MINE-MINE", password)
	assert.Len(t, token, 32, "only the missing key is filled")
}

func TestDashboardCredentialsNeverCreateTheSecret(t *testing.T) {
	t.Setenv(dashboardSecretEnv, "kwatch-dashboard")
	client := fake.NewSimpleClientset()
	token, password := dashboardCredentials(
		context.Background(), client, "kwatch")
	assert.Empty(t, token)
	assert.Empty(t, password)
	_, err := client.CoreV1().Secrets("kwatch").Get(context.Background(),
		"kwatch-dashboard", metav1.GetOptions{})
	require.Error(t, err, "RBAC grants no create; neither does the code")
}

func TestDashboardCredentialsFromEnvironmentForExistingSecret(t *testing.T) {
	t.Setenv(dashboardSecretEnv, "")
	t.Setenv(dashboardTokenEnv, "t0k")
	t.Setenv(dashboardPasswordEnv, "pw")
	token, password := dashboardCredentials(
		context.Background(), fake.NewSimpleClientset(), "kwatch")
	assert.Equal(t, "t0k", token)
	assert.Equal(t, "pw", password)
}
