package app

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/license"
)

func testCLI(
	w *licenseWorld, key string, kube kubernetes.Interface,
) licenseCLI {
	return licenseCLI{
		loadConfig: func() (*config.Config, error) {
			cfg := config.DefaultConfig()
			cfg.License = config.License{Key: key, Endpoint: w.srv.URL}
			return cfg, nil
		},
		kube: func() (kubernetes.Interface, error) { return kube, nil },
		httpClient: func(config.RuntimeConfig) (*http.Client, error) {
			return w.srv.Client(), nil
		},
		verifier: w.verifier(), now: w.clock.Now, namespace: "kwatch",
	}
}

func kubeSystemNS() *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "kube-system", UID: types.UID("uid-kube-system"),
	}}
}

func TestLicenseStatusCLIPrintsTheLocalEvaluation(t *testing.T) {
	w := newLicenseWorld(t)
	key := w.key(license.PlanPro, 30*24*time.Hour)
	var out, errOut bytes.Buffer

	code := testCLI(w, key, nil).status(&out, &errOut)

	require.Zero(t, code)
	require.Empty(t, errOut.String())
	text := out.String()
	require.Contains(t, text, "State:   valid")
	require.Contains(t, text, "Plan:    Pro")
	require.Contains(t, text, "License: lic_…1234")
	require.Contains(t, text, "30 days left")
	require.NotContains(t, text, key, "the key is masked")
	require.Contains(t, text, maskKey(key))
	require.Zero(t, w.total(), "status uses no network")
}

func TestLicenseStatusCLIReportsInvalidExpiredAndMissingKeys(t *testing.T) {
	w := newLicenseWorld(t)
	var out, errOut bytes.Buffer

	require.Zero(t, testCLI(w, "", nil).status(&out, &errOut))
	require.Contains(t, out.String(), "Community")

	out.Reset()
	cli := testCLI(w, "garbage", nil)
	require.Zero(t, cli.status(&out, &errOut))
	require.Contains(t, out.String(), "invalid")

	out.Reset()
	cli = testCLI(w, w.key(license.PlanTrial, time.Hour), nil)
	w.clock.Advance(2 * time.Hour)
	require.Zero(t, cli.status(&out, &errOut))
	require.Contains(t, out.String(), "expired")

	// The compiled-in trust (no keys yet) rejects any key.
	out.Reset()
	cli = testCLI(w, w.key(license.PlanPro, 24*time.Hour), nil)
	cli.verifier = nil
	require.Zero(t, cli.status(&out, &errOut))
	require.Contains(t, out.String(), "invalid")
	require.Zero(t, w.total())
}

func TestLicenseDeactivateCLIReleasesTheBinding(t *testing.T) {
	w := newLicenseWorld(t)
	key := w.key(license.PlanPro, 30*24*time.Hour)
	kube := fake.NewSimpleClientset(kubeSystemNS(), emptyInstanceSecret("kwatch"))
	_, err := license.EnsureInstanceKey(context.Background(),
		kubeInstanceStore{client: kube, namespace: "kwatch"})
	require.NoError(t, err)
	var out, errOut bytes.Buffer

	code := testCLI(w, key, kube).deactivate(
		context.Background(), &out, &errOut)

	require.Zero(t, code, errOut.String())
	require.Equal(t, 1, w.count("/license/deactivate"))
	require.Contains(t, out.String(), "deactivated")
	require.NotContains(t, out.String()+errOut.String(), key)
}

func TestLicenseDeactivateCLIFailsClearly(t *testing.T) {
	w := newLicenseWorld(t)
	key := w.key(license.PlanPro, 30*24*time.Hour)
	kube := fake.NewSimpleClientset(kubeSystemNS(), emptyInstanceSecret("kwatch"))
	_, err := license.EnsureInstanceKey(context.Background(),
		kubeInstanceStore{client: kube, namespace: "kwatch"})
	require.NoError(t, err)

	t.Run("server refuses", func(t *testing.T) {
		w.reject = "revoked"
		defer func() { w.reject = "" }()
		var out, errOut bytes.Buffer
		code := testCLI(w, key, kube).deactivate(
			context.Background(), &out, &errOut)
		require.NotZero(t, code)
		require.Contains(t, errOut.String(), "ERROR: deactivate")
		require.NotContains(t, errOut.String(), key)
	})
	t.Run("no Kubernetes API", func(t *testing.T) {
		cli := testCLI(w, key, kube)
		cli.kube = func() (kubernetes.Interface, error) {
			return nil, context.DeadlineExceeded
		}
		var out, errOut bytes.Buffer
		code := cli.deactivate(context.Background(), &out, &errOut)
		require.NotZero(t, code)
		require.Contains(t, errOut.String(), "inside the kwatch pod")
	})
	t.Run("malformed key", func(t *testing.T) {
		var out, errOut bytes.Buffer
		code := testCLI(w, "nonsense", kube).deactivate(
			context.Background(), &out, &errOut)
		require.NotZero(t, code)
		require.NotContains(t, errOut.String(), "nonsense")
	})
	t.Run("never activated", func(t *testing.T) {
		var out, errOut bytes.Buffer
		code := testCLI(w, key, fake.NewSimpleClientset(kubeSystemNS())).
			deactivate(context.Background(), &out, &errOut)
		require.Zero(t, code)
		require.Contains(t, out.String(), "nothing to deactivate")
	})
	t.Run("no key", func(t *testing.T) {
		var out, errOut bytes.Buffer
		code := testCLI(w, "", kube).deactivate(
			context.Background(), &out, &errOut)
		require.Zero(t, code)
		require.Contains(t, out.String(), "nothing to deactivate")
	})
}

func TestMaskKeyShowsOnlyThePrefixAndTail(t *testing.T) {
	require.Equal(t, "KW1-…wxyz", maskKey("KW1-abcdefghijklmnopqrstuvwxyz"))
	require.Equal(t, "KW1-…", maskKey("KW1-ab"))
}
