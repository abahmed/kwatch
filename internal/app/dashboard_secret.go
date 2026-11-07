package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// dashboardSecretEnv names the Secret that holds the dashboard
// credentials when the chart ships it empty. kwatch fills the keys
// itself, so every renderer (helm template, GitOps controllers, CI)
// sees the same static manifest and the credentials never change.
const dashboardSecretEnv = "KWATCH_DASHBOARD_SECRET"

const (
	dashboardSecretToken    = "token"
	dashboardSecretPassword = "password"
	// passwordAlphabet has no 0/O/1/I/L; 32 letters, so every letter
	// is equally likely from a random byte.
	passwordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	tokenAlphabet    = "abcdefghijklmnopqrstuvwxyz" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	dashboardSecretWait = 15 * time.Second
	secretWriteTries    = 4
)

// dashboardCredentials returns the bearer token and the sign-in
// password. A chart-managed Secret (KWATCH_DASHBOARD_SECRET) is read
// through the API and its missing keys are generated and stored once;
// otherwise the values come from the environment (an existing Secret
// the operator supplies).
func dashboardCredentials(
	ctx context.Context, client kubernetes.Interface, namespace string,
) (token, password string) {
	name := strings.TrimSpace(os.Getenv(dashboardSecretEnv))
	if name == "" || client == nil {
		return os.Getenv(dashboardTokenEnv),
			os.Getenv(dashboardPasswordEnv)
	}
	ctx, cancel := context.WithTimeout(ctx, dashboardSecretWait)
	defer cancel()
	data, err := ensureDashboardSecret(
		ctx, client.CoreV1().Secrets(namespace), name)
	if err != nil {
		klog.ErrorS(err, "dashboard credentials unavailable; every "+
			"dashboard request is refused", "component",
			"dashboard-server", "secret", name)
		return "", ""
	}
	return string(data[dashboardSecretToken]),
		string(data[dashboardSecretPassword])
}

// ensureDashboardSecret fills the token and password of the existing
// Secret when they are missing and returns its data. It only gets and
// updates the one named Secret; it never creates it, because RBAC
// cannot limit create by name. An update conflict (another replica
// started at the same time) is retried by reading again.
func ensureDashboardSecret(
	ctx context.Context, secrets secretClient, name string,
) (map[string][]byte, error) {
	for range secretWriteTries {
		secret, err := secrets.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("secret %q does not exist; the "+
				"chart ships it, or set dashboard.existingSecret", name)
		}
		if err != nil {
			return nil, err
		}
		changed, err := fillDashboardCredentials(secret)
		if err != nil {
			return nil, err
		}
		if !changed {
			return secret.Data, nil
		}
		_, err = secrets.Update(ctx, secret, metav1.UpdateOptions{})
		if err == nil {
			return secret.Data, nil
		}
		if !apierrors.IsConflict(err) {
			return nil, err
		}
	}
	return nil, errors.New("dashboard secret kept changing")
}

// fillDashboardCredentials adds a missing token or password and
// reports whether it changed the Secret.
func fillDashboardCredentials(secret *corev1.Secret) (bool, error) {
	changed := false
	if len(secret.Data[dashboardSecretToken]) == 0 {
		token, err := randomString(tokenAlphabet, 32)
		if err != nil {
			return false, err
		}
		putSecretValue(secret, dashboardSecretToken, token)
		changed = true
	}
	if len(secret.Data[dashboardSecretPassword]) == 0 {
		raw, err := randomString(passwordAlphabet, 16)
		if err != nil {
			return false, err
		}
		groups := []string{
			raw[0:4], raw[4:8], raw[8:12], raw[12:16],
		}
		putSecretValue(secret, dashboardSecretPassword,
			strings.Join(groups, "-"))
		changed = true
	}
	return changed, nil
}

func putSecretValue(secret *corev1.Secret, key, value string) {
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	secret.Data[key] = []byte(value)
}

// randomString draws n characters of alphabet from crypto/rand without
// modulo bias.
func randomString(alphabet string, n int) (string, error) {
	out := make([]byte, n)
	limit := big.NewInt(int64(len(alphabet)))
	for i := range out {
		v, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		out[i] = alphabet[v.Int64()]
	}
	return string(out), nil
}
