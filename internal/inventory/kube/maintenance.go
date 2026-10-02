package kube

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Attributes recording that an object is deliberately under maintenance.
const (
	AttrMaintenance      = "maintenance"
	AttrMaintenanceUntil = "maintenance.until"
)

// MaintenanceAnnotations names the annotations that mark maintenance. The
// zero value disables capture.
type MaintenanceAnnotations struct {
	On    string
	Until string
}

// WithMaintenance makes the translator record maintenance annotations as
// attributes of every object that carries them.
func (t *Translator) WithMaintenance(m MaintenanceAnnotations) *Translator {
	t.maintenance = m
	return t
}

// annotate adds the maintenance attributes of obj to desc.
func (t *Translator) annotate(obj any, desc *Description) {
	m := t.maintenance
	if m.On == "" && m.Until == "" {
		return
	}
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return
	}
	annotations := accessor.GetAnnotations()
	value, on := annotations[m.On]
	raw, until := annotations[m.Until]
	if !(m.On != "" && on) && !(m.Until != "" && until) {
		return
	}
	if desc.Attributes == nil {
		desc.Attributes = make(map[string]inventory.Value)
	}
	if m.On != "" && on {
		desc.Attributes[AttrMaintenance] = inventory.Text(value)
	}
	if m.Until == "" || !until {
		return
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		klog.V(2).InfoS("invalid maintenance timestamp",
			"component", "kube", "operation", "annotate",
			"kind", desc.ID.Kind, "name", desc.ID.Name)
		return
	}
	desc.Attributes[AttrMaintenanceUntil] = inventory.Time(parsed)
}
