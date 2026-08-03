package handler

import (
	"fmt"
	"time"

	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
)

// DetectPdbIssue returns a Signal if the PDB is blocking disruptions.
func DetectPdbIssue(pdb *policyv1.PodDisruptionBudget) *event.Signal {
	if isPdbBlocking(pdb) {
		return &event.Signal{
			Resource:  "poddisruptionbudget",
			Reason:    constant.ReasonPdbViolation,
			Namespace: pdb.Namespace,
			Owner:     pdb.Namespace + "/" + pdb.Name,
			Labels:    pdb.Labels,
			Hint:      pdbHint(pdb),
		}
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
	return fmt.Sprintf("PDB %s/%s blocking: currentHealthy=%d, desiredHealthy=%d — pod disruptions not allowed; check pod health or reduce replica count",
		pdb.Namespace, pdb.Name, pdb.Status.CurrentHealthy, pdb.Status.DesiredHealthy)
}

func (h *handler) ProcessPdb(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid pdb key %q: %w", key, err)
	}

	if deleted {
		h.clearFirstPdbViolation(namespace + "/" + name)
		h.correlator.ResolveByResource("poddisruptionbudget", namespace+"/"+name)
		return nil
	}

	pdb, err := h.pdbLister.PodDisruptionBudgets(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.clearFirstPdbViolation(namespace + "/" + name)
			h.correlator.ResolveByResource("poddisruptionbudget", namespace+"/"+name)
			return nil
		}
		return fmt.Errorf("failed to get pdb %s/%s from cache: %w", namespace, name, err)
	}

	return h.ProcessPdbObject(pdb, false)
}

func (h *handler) ProcessPdbObject(pdb *policyv1.PodDisruptionBudget, deleted bool) error {
	if pdb == nil {
		return nil
	}

	if deleted {
		h.clearFirstPdbViolation(pdb.Namespace + "/" + pdb.Name)
		h.correlator.ResolveByResource("poddisruptionbudget", pdb.Namespace+"/"+pdb.Name)
		return nil
	}

	key := pdb.Namespace + "/" + pdb.Name

	if isPdbBlocking(pdb) {
		first := h.markFirstPdbViolation(key)

		sustained := time.Duration(h.config.PdbMonitor.SustainedMinutes) * time.Minute
		if sustained > 0 && h.now().Sub(first) < sustained {
			return nil
		}

		h.signalEvent(&event.Signal{
			Resource:  "poddisruptionbudget",
			Namespace: pdb.Namespace,
			Reason:    constant.ReasonPdbViolation,
			Owner:     key,
			Labels:    pdb.Labels,
			Hint:      pdbHint(pdb),
		})
		return nil
	}

	h.clearFirstPdbViolation(key)
	h.correlator.ResolveByResource("poddisruptionbudget", key)
	return nil
}

func (h *handler) markFirstPdbViolation(key string) time.Time {
	h.pdbMu.Lock()
	defer h.pdbMu.Unlock()
	if t, ok := h.firstPdbViolation[key]; ok {
		return t
	}
	h.firstPdbViolation[key] = h.now()
	return h.firstPdbViolation[key]
}

func (h *handler) clearFirstPdbViolation(key string) {
	h.pdbMu.Lock()
	defer h.pdbMu.Unlock()
	delete(h.firstPdbViolation, key)
}
