package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestSelectedByTemplateMatchesAWorkloadWithNoPods(t *testing.T) {
	m := inventory.NewModel(inventory.Options{})
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	observe := func(
		kind inventory.Kind, ns, name string,
		attrs map[string]inventory.Value,
	) {
		_, err := m.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: "test", At: at,
			Entity:     inventory.CoreID(kind, ns, name),
			Attributes: attrs,
		})
		assert.NoError(t, err)
	}
	observe(kube.KindDeployment, "shop", "api", map[string]inventory.Value{
		kube.AttrTemplateLabels: inventory.Text("app=api,tier=web"),
	})
	observe(kube.KindDeployment, "other", "db", map[string]inventory.Value{
		kube.AttrTemplateLabels: inventory.Text("app=db"),
	})
	service := func(ns, selector string) inventory.Entity {
		observe(kube.KindService, ns, "svc", map[string]inventory.Value{
			kube.AttrSelector: inventory.Text(selector),
		})
		e, _ := m.Entity(inventory.CoreID(kube.KindService, ns, "svc"))
		return e
	}

	assert.True(t, kube.SelectedByTemplate(m, service("shop", "app=api")))
	assert.False(t, kube.SelectedByTemplate(m, service("shop", "app=apii")))
	assert.False(t, kube.SelectedByTemplate(m, service("shop", "app=db")),
		"a template in another namespace is not a backend")
	assert.False(t, kube.SelectedByTemplate(m, service("shop", "")))
}
