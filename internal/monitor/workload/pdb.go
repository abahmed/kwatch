package workload

import (
	"fmt"

	policyv1 "k8s.io/api/policy/v1"
	policyv1lister "k8s.io/client-go/listers/policy/v1"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// DetectPDBIssue returns a finding when a PDB blocks disruptions.
func DetectPDBIssue(
	pdb *policyv1.PodDisruptionBudget,
) *model.Observation {
	if pdb == nil || !PDBBlocking(pdb) {
		return nil
	}
	return observe.Object(
		"poddisruptionbudget", pdb, constant.ReasonPdbViolation,
	).WithHint(PDBHint(pdb))
}

// PDBBlocking reports whether a PDB has fewer healthy pods than required.
func PDBBlocking(pdb *policyv1.PodDisruptionBudget) bool {
	return pdb != nil &&
		pdb.Status.ObservedGeneration >= pdb.Generation &&
		pdb.Status.DesiredHealthy > 0 &&
		pdb.Status.DisruptionsAllowed == 0 &&
		pdb.Status.CurrentHealthy < pdb.Status.DesiredHealthy
}

// PDBHint summarizes the health deficit that blocks disruptions.
func PDBHint(pdb *policyv1.PodDisruptionBudget) string {
	if pdb == nil {
		return ""
	}
	return fmt.Sprintf(
		"PDB %s/%s blocking: currentHealthy=%d, desiredHealthy=%d — pod "+
			"disruptions not allowed; check pod health or reduce replica count",
		pdb.Namespace,
		pdb.Name,
		pdb.Status.CurrentHealthy,
		pdb.Status.DesiredHealthy,
	)
}

// PDBProcessor is the controller-facing contract for PodDisruptionBudgets.
type PDBProcessor interface {
	ProcessPdb(string, bool) error
}

// PDBConfig wires informer and clock dependencies into a direct runtime.
type PDBConfig interface {
	SetLister(policyv1lister.PodDisruptionBudgetLister)
}

// PDBRuntime owns PDB lookup and sustained blocking policy.
type PDBRuntime struct {
	support runtimeSupport
	lister  policyv1lister.PodDisruptionBudgetLister
	first   firstSeen
}

// NewPDBRuntime constructs the direct PDB family adapter.
// SetLister supplies the informer-backed PDB cache.
func (r *PDBRuntime) SetLister(
	lister policyv1lister.PodDisruptionBudgetLister,
) {
	r.support.configureSource(func() { r.lister = lister })
}

// ProcessPdb reconciles one PDB queue key.
func (r *PDBRuntime) ProcessPdb(key string, deleted bool) error {
	return processKey(
		&r.support, key, "poddisruptionbudget", deleted,
		nil,
		func() (policyv1lister.PodDisruptionBudgetLister, bool) {
			lister := r.support.sourceSnapshot(
				func() policyv1lister.PodDisruptionBudgetLister {
					return r.lister
				},
			)
			return lister, lister != nil
		},
		func(
			lister policyv1lister.PodDisruptionBudgetLister,
			namespace, name string,
		) (*policyv1.PodDisruptionBudget, error) {
			return lister.PodDisruptionBudgets(namespace).Get(name)
		},
		func(subject model.ObjectRef) {
			r.clearFirst(subject.Namespace + "/" + subject.Name)
			r.support.reconcileGone(subject)
		},
		func(
			subject model.ObjectRef,
			pdb *policyv1.PodDisruptionBudget,
		) error {
			return r.processPDBObject(subject, pdb)
		},
	)
}

func (r *PDBRuntime) processPDBObject(
	subject model.ObjectRef,
	pdb *policyv1.PodDisruptionBudget,
) error {
	if pdb == nil {
		return nil
	}
	key := pdb.Namespace + "/" + pdb.Name
	if r.support.maintenance(pdb.Annotations) {
		r.clearFirst(key)
		r.support.reconcileGone(subject)
		return nil
	}
	if !PDBBlocking(pdb) {
		r.clearFirst(key)
		r.support.reconcile(subject, nil)
		return nil
	}
	first := r.first.mark(key, r.support.now())
	sustained := adaptiveSustained(
		r.support.runtime.PdbMonitor().SustainedMinutes,
		r.support.runtime.AdaptiveThresholds(),
		pdb.Status.DesiredHealthy,
		pdb.Status.DesiredHealthy-pdb.Status.CurrentHealthy,
	)
	if sustained > 0 && r.support.now().Sub(first) < sustained {
		return nil
	}
	r.support.reconcile(subject, observations(DetectPDBIssue(pdb)))
	return nil
}

func (r *PDBRuntime) clearFirst(key string) {
	r.first.clear(key)
}
