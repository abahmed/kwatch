package crd

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"gopkg.in/yaml.v3"

	"github.com/abahmed/kwatch/internal/config"
)

// ApplyStartupConfigWithClient applies the startup overlay using the shared
// application-owned dynamic client.
func ApplyStartupConfigWithClient(
	ctx context.Context,
	cfg *config.Config,
	dynamicClient dynamic.Interface,
	namespace string,
) error {
	return applyStartupConfig(ctx, cfg, dynamicClient, namespace)
}

// ErrInvalidOverlay marks a KwatchConfig that cannot be applied: more than
// one object, a forbidden field, or a spec that fails validation. Callers
// keep the mounted configuration instead of failing, so a bad edit cannot
// crash-loop kwatch.
var ErrInvalidOverlay = stderrors.New("KwatchConfig is invalid")

// forbiddenSpecPaths are settings a KwatchConfig may not change. Outbound
// transport, TLS trust, diagnostics exposure and file paths change who can
// observe provider credentials; they stay in the mounted config, which
// needs more privilege to edit than a KwatchConfig. The CRD schema omits
// them too.
var forbiddenSpecPaths = [][]string{
	{"heartbeatMonitor", "url"},
	{"app", "proxyURL"},
	{"app", "insecureSkipTLSVerify"},
	{"app", "caBundlePath"},
	{"kubelet", "insecureSkipVerify"},
	{"auditLog", "output"},
}

func applyStartupConfig(
	ctx context.Context,
	cfg *config.Config,
	dynamicClient dynamic.Interface,
	namespace string,
) error {
	if !config.RuntimeConfigFor(cfg).Lifecycle().CRDEnabled() {
		return nil
	}
	dc := dynamicClient
	if dc == nil {
		return fmt.Errorf("crdwatch: dynamic client is not configured")
	}
	list, err := dc.Resource(gvr).Namespace(namespace).List(
		ctx, metav1.ListOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if len(list.Items) == 0 {
		return nil
	}
	if len(list.Items) != 1 {
		return fmt.Errorf(
			"%w: expected at most one KwatchConfig in namespace %q",
			ErrInvalidOverlay, namespace,
		)
	}
	return ApplyOverlay(cfg, &list.Items[0])
}

// ApplyOverlay applies one KwatchConfig over cfg and revalidates it. Every
// failure wraps ErrInvalidOverlay. On failure cfg is partly overlaid and
// must be discarded.
func ApplyOverlay(cfg *config.Config, obj *unstructured.Unstructured) error {
	spec, ok := obj.Object["spec"]
	if !ok {
		return nil
	}
	if err := applySpec(cfg, spec); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidOverlay, err)
	}
	return nil
}

func applySpec(cfg *config.Config, spec interface{}) error {
	if err := rejectSecretConfig(spec); err != nil {
		return err
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	config.ResetDerivedSilences(cfg)
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return err
	}
	// KwatchConfig typos are reported with the file's unknown keys.
	config.NoteOverlayUnknownKeys(cfg, "KwatchConfig spec", raw)
	return config.RebuildAfterOverlay(cfg)
}

func rejectSecretConfig(spec interface{}) error {
	root, ok := spec.(map[string]interface{})
	if !ok {
		return nil
	}
	if _, exists := root["alert"]; exists {
		return stderrors.New(
			"KwatchConfig.spec.alert is forbidden; mount provider " +
				"credentials through config.yaml file references",
		)
	}
	for _, path := range forbiddenSpecPaths {
		section, ok := root[path[0]].(map[string]interface{})
		if !ok {
			continue
		}
		if _, exists := section[path[1]]; exists {
			return fmt.Errorf(
				"KwatchConfig.spec.%s.%s is forbidden; configure it "+
					"in the mounted configuration",
				path[0],
				path[1],
			)
		}
	}
	return nil
}
