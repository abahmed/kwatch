package app

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	// licenseInstanceSecret holds this installation's license instance
	// key. The chart's RBAC names it.
	licenseInstanceSecret = "kwatch-license-instance"
	// licenseInstanceKeyField is the Secret key holding the raw ed25519
	// seed.
	licenseInstanceKeyField = "ed25519"
	// licenseDeactivatedField holds the RFC 3339 time `kwatch license
	// deactivate` last released this cluster. The CLI and the leader are
	// separate processes, so the Secret is how the leader learns of it.
	licenseDeactivatedField = "deactivated-at"
)

// State keys of the license in the state bucket.
const (
	stateLicenseLease    = "license.lease"
	stateLicenseClock    = "license.clock"
	stateLicenseNotified = "license.notified"
	// stateLicenseVersion is the kwatch version last reported to the
	// license server, so an upgrade renews at once.
	stateLicenseVersion = "license.reported_version"
	// stateLicenseRenewAttempt is a pending early renewal for a new
	// version, so restarts respect its retry window.
	stateLicenseRenewAttempt = "license.renew_attempt"
)

// kubeInstanceStore keeps the instance key in one Secret that the
// manifests ship pre-created and empty. It reads and updates only that
// name and never creates it: RBAC cannot limit create by resourceNames, so
// granting create would open every Secret in the namespace.
type kubeInstanceStore struct {
	client    kubernetes.Interface
	namespace string
}

func (s kubeInstanceStore) secrets() secretClient {
	return s.client.CoreV1().Secrets(s.namespace)
}

// secretClient is the part of the Secret API the store uses.
type secretClient interface {
	Get(context.Context, string, metav1.GetOptions) (*corev1.Secret, error)
	Update(context.Context, *corev1.Secret, metav1.UpdateOptions) (
		*corev1.Secret, error)
}

// Load implements license.InstanceKeyStore. A missing Secret, an empty
// shipped Secret with no ed25519 key, or one whose key is malformed reads
// as no key: the manager then makes a new one and Save fills the Secret.
func (s kubeInstanceStore) Load(ctx context.Context) (
	ed25519.PrivateKey, bool, error,
) {
	secret, err := s.secrets().Get(
		ctx, licenseInstanceSecret, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	seed := secret.Data[licenseInstanceKeyField]
	if len(seed) != ed25519.SeedSize {
		return nil, false, nil
	}
	return ed25519.NewKeyFromSeed(seed), true, nil
}

// Save implements license.InstanceKeyStore. It updates the existing Secret
// and fails, without creating it, when the Secret does not exist.
func (s kubeInstanceStore) Save(
	ctx context.Context, key ed25519.PrivateKey,
) error {
	if len(key) != ed25519.PrivateKeySize {
		return errors.New("license instance key has the wrong size")
	}
	existing, err := s.secrets().Get(
		ctx, licenseInstanceSecret, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		err = fmt.Errorf("secret %s/%s does not exist: create it "+
			"empty (type Opaque) and grant this ServiceAccount get and "+
			"update on it with resourceNames [%s]; kwatch never "+
			"creates it, so the license stays pending",
			s.namespace, licenseInstanceSecret, licenseInstanceSecret)
		klog.ErrorS(err, "license instance Secret missing",
			"component", "license")
		return err
	}
	if err != nil {
		return err
	}
	if existing.Data == nil {
		existing.Data = map[string][]byte{}
	}
	existing.Data[licenseInstanceKeyField] = key.Seed()
	_, err = s.secrets().Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

// MarkDeactivated records that this cluster was just released, for the
// leader to pick up.
func (s kubeInstanceStore) MarkDeactivated(
	ctx context.Context, at time.Time,
) error {
	existing, err := s.secrets().Get(
		ctx, licenseInstanceSecret, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if existing.Data == nil {
		existing.Data = map[string][]byte{}
	}
	existing.Data[licenseDeactivatedField] = []byte(
		at.UTC().Format(time.RFC3339))
	_, err = s.secrets().Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

// DeactivatedAt returns the time of the last release, zero when there is
// none or the Secret does not exist.
func (s kubeInstanceStore) DeactivatedAt(
	ctx context.Context,
) (time.Time, error) {
	secret, err := s.secrets().Get(
		ctx, licenseInstanceSecret, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	raw := secret.Data[licenseDeactivatedField]
	if len(raw) == 0 {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, string(raw))
}

// kubeSystemUID reads the kube-system namespace UID live. It is never
// cached by the caller into a proof.
func kubeSystemUID(
	client kubernetes.Interface,
) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		ns, err := client.CoreV1().Namespaces().Get(
			ctx, "kube-system", metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		if ns.UID == "" {
			return "", errors.New("kube-system namespace has no UID")
		}
		return string(ns.UID), nil
	}
}

// nodeCount counts nodes with a one-item list: the API server reports the
// remaining count, so no full node list is transferred. A failure counts
// zero; the count is advisory.
func nodeCount(client kubernetes.Interface) func(context.Context) int {
	return func(ctx context.Context) int {
		list, err := client.CoreV1().Nodes().List(
			ctx, metav1.ListOptions{Limit: 1})
		if err != nil {
			return 0
		}
		n := len(list.Items)
		if list.RemainingItemCount != nil {
			n += int(*list.RemainingItemCount)
		}
		return n
	}
}
