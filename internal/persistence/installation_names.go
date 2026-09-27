package persistence

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// defaultStatePrefix is the historical name prefix of every state ConfigMap.
const defaultStatePrefix = "kwatch"

// Option configures a Manager.
type Option func(*managerSettings)

type managerSettings struct {
	prefix string
}

// WithStatePrefix scopes state ConfigMaps to one installation, so several
// installations in one namespace no longer share and overwrite each other's
// state. The default prefix keeps the historical kwatch-* names.
func WithStatePrefix(prefix string) Option {
	return func(settings *managerSettings) {
		prefix = strings.TrimSpace(prefix)
		if prefix == defaultStatePrefix {
			prefix = ""
		}
		settings.prefix = prefix
	}
}

// statePrefix maps a logical kwatch-* name to the installation's name.
func statePrefix(prefix string) func(string) string {
	return func(logical string) string {
		if prefix == "" {
			return logical
		}
		return prefix + strings.TrimPrefix(logical, defaultStatePrefix)
	}
}

func (s *Manager) name(logical string) string {
	return statePrefix(s.prefix)(logical)
}

// getConfigMap reads the installation's ConfigMap. When an installation has
// just moved to a scoped name, the first read falls back to the historical
// name so existing state is carried over; the next write lands on the scoped
// name.
func (s *Manager) getConfigMap(
	ctx context.Context, logical string,
) (*corev1.ConfigMap, error) {
	configMaps := s.client.CoreV1().ConfigMaps(s.namespace)
	cm, err := configMaps.Get(ctx, s.name(logical), metav1.GetOptions{})
	if apierrors.IsNotFound(err) && s.name(logical) != logical {
		return configMaps.Get(ctx, logical, metav1.GetOptions{})
	}
	return cm, err
}

func (s *Manager) shardPrefix() string {
	return s.name(incidentShardPrefix)
}
