package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/license"
	"github.com/abahmed/kwatch/internal/version"
)

// licenseCLITimeout bounds `kwatch license deactivate` as a whole.
const licenseCLITimeout = 30 * time.Second

// licenseCLI holds what the license subcommands read from the outside, so
// tests can replace it.
type licenseCLI struct {
	loadConfig func() (*config.Config, error)
	kube       func() (kubernetes.Interface, error)
	httpClient func(config.RuntimeConfig) (*http.Client, error)
	verifier   *license.Verifier
	now        func() time.Time
	namespace  string
}

func defaultLicenseCLI() licenseCLI {
	return licenseCLI{
		loadConfig: config.LoadConfig,
		kube:       inClusterKube,
		httpClient: kubeclient.NewHTTPClientWithRuntime,
		now:        clock.RealClock{}.Now,
		namespace:  kubeclient.GetNamespace(),
	}
}

// inClusterKube connects with the pod's service account only, never a
// kubeconfig: the command releases the license of the cluster it runs in.
func inClusterKube() (kubernetes.Interface, error) {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(restConfig)
}

// LicenseStatus implements `kwatch license status`: the local evaluation
// of the configured key, with no network access. The key is shown masked.
func LicenseStatus(out, errOut io.Writer) int {
	return defaultLicenseCLI().status(out, errOut)
}

// LicenseDeactivate implements `kwatch license deactivate`: it releases
// this cluster's binding on the license server. It runs inside the kwatch
// pod, where the in-cluster configuration and the instance Secret exist.
func LicenseDeactivate(out, errOut io.Writer) int {
	ctx, cancel := context.WithTimeout(
		context.Background(), licenseCLITimeout)
	defer cancel()
	return defaultLicenseCLI().deactivate(ctx, out, errOut)
}

// maskKey shows the key prefix and its last four characters only.
func maskKey(key string) string {
	const prefix = "KW1-"
	if len(key) <= len(prefix)+4 {
		return prefix + "…"
	}
	return prefix + "…" + key[len(key)-4:]
}

func (c licenseCLI) status(out, errOut io.Writer) int {
	cfg, err := c.loadConfig()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "ERROR: load configuration: %v\n", err)
		return 1
	}
	key := cfg.License.Key
	if key == "" {
		_, _ = fmt.Fprintln(out,
			"No license key is configured: kwatch runs the Community edition.")
		return 0
	}
	verifier := c.verifier
	if verifier == nil {
		verifier = license.DefaultVerifier()
	}
	now := c.now()
	_, _ = fmt.Fprintf(out, "Key:     %s\n", maskKey(key))
	parsed, parseErr := license.ParseKey(key)
	verified, verifyErr := verifier.VerifyKey(key, now)
	switch {
	case parseErr != nil:
		_, _ = fmt.Fprintf(out, "State:   invalid (%v)\n", parseErr)
		return 0
	case verifyErr != nil && !errors.Is(verifyErr, license.ErrExpired):
		_, _ = fmt.Fprintf(out, "State:   invalid (%v)\n", verifyErr)
		return 0
	case verifyErr != nil:
		_, _ = fmt.Fprintln(out, "State:   expired")
	default:
		_, _ = fmt.Fprintln(out, "State:   valid key (activation is not "+
			"checked here)")
	}
	_, _ = fmt.Fprintf(out, "Plan:    %s\n", verified.Plan)
	_, _ = fmt.Fprintf(out, "License: %s\n", license.MaskLID(parsed.LID))
	expires := parsed.ExpiresAt()
	days := int(expires.Sub(now) / (24 * time.Hour))
	_, _ = fmt.Fprintf(out, "Expires: %s (%d days left)\n",
		expires.Format("2006-01-02"), max(days, 0))
	return 0
}

func (c licenseCLI) deactivate(
	ctx context.Context, out, errOut io.Writer,
) int {
	cfg, err := c.loadConfig()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "ERROR: load configuration: %v\n", err)
		return 1
	}
	if cfg.License.Key == "" {
		_, _ = fmt.Fprintln(out,
			"No license key is configured; nothing to deactivate.")
		return 0
	}
	message, err := c.release(ctx, cfg)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "ERROR: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintln(out, message)
	return 0
}

// release asks the license server to free this cluster's binding. Its
// errors never contain the key.
func (c licenseCLI) release(
	ctx context.Context, cfg *config.Config,
) (string, error) {
	key, err := license.ParseKey(cfg.License.Key)
	if err != nil {
		return "", fmt.Errorf(
			"the configured license key is not valid: %v", err)
	}
	kube, err := c.kube()
	if err != nil {
		return "", fmt.Errorf("connect to the Kubernetes API (run this "+
			"inside the kwatch pod): %v", err)
	}
	priv, found, err := kubeInstanceStore{
		client: kube, namespace: c.namespace,
	}.Load(ctx)
	if err != nil {
		return "", fmt.Errorf("read the instance key: %v", err)
	}
	if !found {
		return "This cluster was never activated with this license; " +
			"nothing to deactivate.", nil
	}
	uid, err := kubeSystemUID(kube)(ctx)
	if err != nil {
		return "", fmt.Errorf("read the kube-system namespace: %v", err)
	}
	httpClient, err := c.httpClient(config.RuntimeConfigFor(cfg))
	if err != nil {
		return "", fmt.Errorf("outbound HTTP client: %v", err)
	}
	client := &license.Client{
		Endpoint: cfg.License.Endpoint, HTTP: httpClient,
		UserAgent: "kwatch-license/1",
	}
	_, err = client.Deactivate(ctx, priv, license.SignedRequest{
		LID:          key.LID,
		ClusterUUID:  clusterIDFromUID(uid),
		ClusterProof: license.ClusterProof(uid, key.LID),
		TS:           c.now(),
		Version:      version.Short(),
		NodeCount:    nodeCount(kube)(ctx),
	})
	if err != nil && !errors.Is(err, license.ErrUnauthorized) {
		return "", fmt.Errorf("deactivate %s: %v",
			license.MaskLID(key.LID), err)
	}
	msg := fmt.Sprintf("License %s is deactivated for this cluster.",
		license.MaskLID(key.LID))
	// Tell the running leader, a separate process, to drop its lease.
	if err := (kubeInstanceStore{client: kube, namespace: c.namespace}).
		MarkDeactivated(ctx, c.now()); err != nil {
		msg += fmt.Sprintf("\nWarning: could not tell the running kwatch "+
			"(%v); restart the pod so it stops using the old lease.", err)
	}
	return msg, nil
}
