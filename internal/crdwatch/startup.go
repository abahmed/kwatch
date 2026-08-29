package crdwatch

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"gopkg.in/yaml.v3"

	"github.com/abahmed/kwatch/internal/config"
)

// ApplyStartupConfig overlays the optional KwatchConfig before components are
// constructed. A CRD is deliberately a startup-only source; live changes
// cause a restart through Watcher.
func ApplyStartupConfig(ctx context.Context, cfg *config.Config, restCfg *rest.Config, namespace string) error {
	if !cfg.CrdConfig.Enabled {
		return nil
	}
	dc, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return err
	}
	list, err := dc.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if len(list.Items) == 0 {
		return nil
	}
	if len(list.Items) != 1 {
		return fmt.Errorf("expected at most one KwatchConfig in namespace %q", namespace)
	}
	spec, ok := list.Items[0].Object["spec"]
	if !ok {
		return nil
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	config.ResetDerivedSilences(cfg)
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return err
	}
	return config.RebuildAfterOverlay(cfg)
}
