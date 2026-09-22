//go:build e2e

package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
)

func (e *Environment) ApplyFixture(
	ctx context.Context,
	path string,
) error {
	payload, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}
	return e.ApplyYAML(ctx, payload)
}

func (e *Environment) ApplyYAML(
	ctx context.Context,
	payload []byte,
) error {
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(
		memory.NewMemCacheClient(e.Discovery),
	)
	decoder := utilyaml.NewYAMLOrJSONDecoder(
		bytes.NewReader(payload), 4096,
	)
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("decode fixture: %w", err)
		}
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		object := &unstructured.Unstructured{}
		if err := json.Unmarshal(raw, object); err != nil {
			return fmt.Errorf("decode fixture object: %w", err)
		}
		if err := applyObject(ctx, e.Dynamic, mapper, object); err != nil {
			return err
		}
	}
}

func applyObject(
	ctx context.Context,
	client dynamic.Interface,
	mapper *restmapper.DeferredDiscoveryRESTMapper,
	object *unstructured.Unstructured,
) error {
	gv, err := schema.ParseGroupVersion(object.GetAPIVersion())
	if err != nil {
		return fmt.Errorf("parse fixture apiVersion: %w", err)
	}
	mapping, err := mapper.RESTMapping(
		schema.GroupKind{Group: gv.Group, Kind: object.GetKind()}, gv.Version,
	)
	if err != nil {
		return fmt.Errorf("map fixture kind %s: %w", object.GetKind(), err)
	}
	resource := client.Resource(mapping.Resource)
	var target dynamic.ResourceInterface = resource
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		target = resource.Namespace(object.GetNamespace())
	}
	name := object.GetName()
	created, err := target.Create(ctx, object, metav1.CreateOptions{})
	if err == nil {
		_ = created
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create fixture %s/%s: %w", object.GetKind(), name, err)
	}
	existing, err := target.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf(
			"get existing fixture %s/%s: %w", object.GetKind(), name, err,
		)
	}
	object.SetResourceVersion(existing.GetResourceVersion())
	if _, err := target.Update(ctx, object, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update fixture %s/%s: %w", object.GetKind(), name, err)
	}
	return nil
}
