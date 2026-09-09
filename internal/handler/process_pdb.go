package handler

import (
	"fmt"
	"time"

	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// DetectPdbIssue returns a Signal if the PDB is blocking disruptions.
func DetectPdbIssue(
	pdb *policyv1.PodDisruptionBudget,
) *model.Observation {
	if pdb == nil {
		return nil
	}
	if isPdbBlocking(pdb) {
		return observe.Object(
			"poddisruptionbudget", pdb, constant.ReasonPdbViolation,
		).WithHint(pdbHint(pdb))
	}
	return nil
}

func isPdbBlocking(pdb *policyv1.PodDisruptionBudget) bool {
	return pdb.Status.ObservedGeneration >= pdb.Generation &&
		pdb.Status.DesiredHealthy > 0 &&
		pdb.Status.DisruptionsAllowed == 0 &&
		pdb.Status.CurrentHealthy < pdb.Status.DesiredHealthy
}

func pdbHint(pdb *policyv1.PodDisruptionBudget) string {
	return fmt.Sprintf(
		"PDB %s/%s blocking: currentHealthy=%d, desiredHealthy=%d — pod "+
			"disruptions not allowed; check pod health or reduce replica count",
		pdb.Namespace,
		pdb.Name,
		pdb.Status.CurrentHealthy,
		pdb.Status.DesiredHealthy,
	)
}

func (h *handler) ProcessPdb(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid pdb key %q: %w", key, err)
	}

	subject := model.NewObjectRef("poddisruptionbudget", namespace, name)
	if deleted {
		h.clearFirstPdbViolation(namespace + "/" + name)
		h.reconcileGone(subject)
		return nil
	}
	pdb, err := h.listers.PDB.PodDisruptionBudgets(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.clearFirstPdbViolation(namespace + "/" + name)
			h.reconcileGone(subject)
			return nil
		}
		return fmt.Errorf(
			"failed to get pdb %s/%s from cache: %w",
			namespace,
			name,
			err,
		)
	}

	return h.ProcessPdbObject(pdb, false)
}

func (h *handler) ProcessPdbObject(
	pdb *policyv1.PodDisruptionBudget,
	deleted bool,
) error {
	if pdb == nil {
		return nil
	}

	subject := model.NewObjectRef(
		"poddisruptionbudget", pdb.Namespace, pdb.Name,
	)
	key := pdb.Namespace + "/" + pdb.Name
	if deleted || h.inMaintenance(pdb.Annotations) {
		h.clearFirstPdbViolation(key)
		h.reconcileGone(subject)
		return nil
	}

	if !isPdbBlocking(pdb) {
		h.clearFirstPdbViolation(key)
		h.reconcile(subject, nil)
		return nil
	}

	first := h.markFirstPdbViolation(key)
	sustained := adaptiveSustained(
		h.config.PdbMonitor.SustainedMinutes,
		h.config.AdaptiveThresholds,
		pdb.Status.DesiredHealthy,
		pdb.Status.DesiredHealthy-pdb.Status.CurrentHealthy,
	)
	if sustained > 0 && h.now().Sub(first) < sustained {
		return nil
	}

	h.reconcile(subject, findings(observe.Object(
		"poddisruptionbudget", pdb, constant.ReasonPdbViolation,
	).WithHint(pdbHint(pdb))))
	return nil
}

func (h *handler) markFirstPdbViolation(key string) time.Time {
	return h.fs.pdbViolation.mark(key, h.now())
}

func (h *handler) clearFirstPdbViolation(key string) {
	h.fs.pdbViolation.clear(key)
}
