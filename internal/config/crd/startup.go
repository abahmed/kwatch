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
// application-owned dynamic client. It returns the spec (as JSON) of the
// KwatchConfig it found, applied or not, and nil when there is none, so
// the caller can remember the last spec that applied cleanly.
func ApplyStartupConfigWithClient(
	ctx context.Context,
	cfg *config.Config,
	dynamicClient dynamic.Interface,
	namespace string,
) ([]byte, error) {
	return applyStartupConfig(ctx, cfg, dynamicClient, namespace)
}

// ErrInvalidOverlay marks a KwatchConfig that cannot be applied: more than
// one object, a forbidden field, or a spec that fails validation. Callers
// keep the mounted configuration instead of failing, so a bad edit cannot
// crash-loop kwatch.
var ErrInvalidOverlay = stderrors.New("KwatchConfig is invalid")

func applyStartupConfig(
	ctx context.Context,
	cfg *config.Config,
	dynamicClient dynamic.Interface,
	namespace string,
) ([]byte, error) {
	if !config.RuntimeConfigFor(cfg).Lifecycle().CRDEnabled() {
		return nil, nil
	}
	dc := dynamicClient
	if dc == nil {
		return nil, fmt.Errorf("crdwatch: dynamic client is not configured")
	}
	list, err := dc.Resource(gvr).Namespace(namespace).List(
		ctx, metav1.ListOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(list.Items) == 0 {
		return nil, nil
	}
	if len(list.Items) != 1 {
		return nil, fmt.Errorf(
			"%w: expected at most one KwatchConfig in namespace %q",
			ErrInvalidOverlay, namespace,
		)
	}
	spec := specJSON(&list.Items[0])
	return spec, ApplyOverlay(cfg, &list.Items[0])
}

// specJSON is the spec of obj as JSON, or nil when it has none.
func specJSON(obj *unstructured.Unstructured) []byte {
	spec, ok := obj.Object["spec"]
	if !ok {
		return nil
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil
	}
	return raw
}

// ApplySavedSpec applies a spec saved from an earlier start (see
// ApplyStartupConfigWithClient) over cfg, with every rule of a live
// KwatchConfig. Every failure wraps ErrInvalidOverlay and cfg must then be
// discarded.
func ApplySavedSpec(cfg *config.Config, spec []byte) error {
	var decoded interface{}
	if err := json.Unmarshal(spec, &decoded); err != nil {
		return fmt.Errorf("%w: saved spec: %w", ErrInvalidOverlay, err)
	}
	return ApplyOverlay(cfg, &unstructured.Unstructured{
		Object: map[string]interface{}{"spec": decoded}})
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
	if err := rejectForbiddenSettings(spec); err != nil {
		return err
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	mounted := snapshotMounted(cfg)
	config.ResetDerivedSilences(cfg)
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return err
	}
	// KwatchConfig typos are reported with the file's unknown keys.
	config.NoteOverlayUnknownKeys(cfg, "KwatchConfig spec", raw)
	if err := mounted.guard(cfg); err != nil {
		return err
	}
	return config.RebuildAfterOverlay(cfg)
}
