package kube

import (
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Attribute names for image pull credentials.
const (
	// AttrPullSecrets lists the Secrets a pod names in imagePullSecrets,
	// comma separated, in spec order. The admission controller copies a
	// ServiceAccount's pull secrets into the pod, so this is every
	// secret the kubelet tries.
	AttrPullSecrets = "image.pull.secrets"
	// AttrChanged is when a registry login Secret last changed: its
	// newest spec write, else its creation. Only the Secret's metadata
	// is read; a value is never involved.
	AttrChanged = "changed"
)

// setSpecAttributes records what the pod's spec says about where and how
// it runs: its scheduling constraints and its image pull Secrets.
func setSpecAttributes(attrs map[string]inventory.Value, pod *corev1.Pod) {
	setSchedulingSpec(attrs, pod)
	setPullSecrets(attrs, pod)
}

// setPullSecrets records the pod's image pull Secret names.
func setPullSecrets(attrs map[string]inventory.Value, pod *corev1.Pod) {
	var names []string
	for _, ref := range pod.Spec.ImagePullSecrets {
		if ref.Name != "" {
			names = append(names, ref.Name)
		}
	}
	if len(names) > 0 {
		attrs[AttrPullSecrets] = inventory.Text(strings.Join(names, ","))
	}
}

// PullSecretNames splits the AttrPullSecrets value of a pod.
func PullSecretNames(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

// registryLoginTypes are the Secret types that hold registry logins.
var registryLoginTypes = map[corev1.SecretType]bool{
	corev1.SecretTypeDockerConfigJson: true,
	corev1.SecretTypeDockercfg:        true,
}

// setLoginChanged records when a registry login Secret last changed.
// Other Secrets carry no such attribute.
func setLoginChanged(attrs map[string]inventory.Value, s *corev1.Secret) {
	if !registryLoginTypes[s.Type] {
		return
	}
	changed := latestSpecTime(s)
	if changed.IsZero() {
		changed = s.CreationTimestamp.Time
	}
	if !changed.IsZero() {
		attrs[AttrChanged] = inventory.Time(changed)
	}
}

// refusalPatterns find the registry's own words in a pull error, the
// most specific first: the distribution spec's error codes, then the
// plain phrases registries and runtimes use. The match is quoted as it
// stands.
var refusalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)unauthorized: (?:authentication required|` +
		`incorrect username or password|[a-z ]*credentials[a-z ]*)`),
	regexp.MustCompile(`(?i)unauthorized: [^":;]+`),
	regexp.MustCompile(`(?i)denied: [^":;]+`),
	regexp.MustCompile(`(?i)pull access denied[^":;]*`),
	regexp.MustCompile(`(?i)no basic auth credentials`),
	regexp.MustCompile(`(?i)authentication required`),
	regexp.MustCompile(`(?i)insufficient_scope[^":;]*`),
	regexp.MustCompile(`(?i)40[13] (?:Unauthorized|Forbidden)`),
}

// PullRefusal returns the registry's own refusal in a pull error
// message, or "" when it holds none. Event notes carry it, since their
// message is cut before the end where the refusal is.
func PullRefusal(message string) string {
	for _, pattern := range refusalPatterns {
		found := strings.TrimSpace(pattern.FindString(message))
		if found != "" {
			return evidenceText(found)
		}
	}
	return ""
}
