package controller

import (
	"errors"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// errNoLister means this controller does not watch the kind, so absence from
// its cache is not evidence the object is gone.
var errNoLister = errors.New("no lister for resource")

// existsFn asks one lister whether an object is present. It returns only the
// error: NotFound means gone, errNoLister means "cannot answer", and anything
// else is a cache problem that must not be read as deletion.
type existsFn func(c *Controller, namespace, name string) error

// resourceExistsTable is a flat dispatch table rather than a switch: one entry
// per watched kind keeps each lookup trivial and the dispatch itself
// branchless.
var resourceExistsTable = map[string]existsFn{
	"pod": func(c *Controller, ns, name string) error {
		if c.podLister == nil {
			return errNoLister
		}
		_, err := c.podLister.Pods(ns).Get(name)
		return err
	},
	"node": func(c *Controller, _, name string) error {
		if c.nodeLister == nil {
			return errNoLister
		}
		_, err := c.nodeLister.Get(name)
		return err
	},
	"deployment": func(c *Controller, ns, name string) error {
		if c.deployLister == nil {
			return errNoLister
		}
		_, err := c.deployLister.Deployments(ns).Get(name)
		return err
	},
	"statefulset": func(c *Controller, ns, name string) error {
		if c.ssLister == nil {
			return errNoLister
		}
		_, err := c.ssLister.StatefulSets(ns).Get(name)
		return err
	},
	"daemonset": func(c *Controller, ns, name string) error {
		if c.dsLister == nil {
			return errNoLister
		}
		_, err := c.dsLister.DaemonSets(ns).Get(name)
		return err
	},
	"replicaset": func(c *Controller, ns, name string) error {
		if c.rsLister == nil {
			return errNoLister
		}
		_, err := c.rsLister.ReplicaSets(ns).Get(name)
		return err
	},
	"job": func(c *Controller, ns, name string) error {
		if c.jobLister == nil {
			return errNoLister
		}
		_, err := c.jobLister.Jobs(ns).Get(name)
		return err
	},
	"cronjob": func(c *Controller, ns, name string) error {
		if c.cronJobLister == nil {
			return errNoLister
		}
		_, err := c.cronJobLister.CronJobs(ns).Get(name)
		return err
	},
	"horizontalpodautoscaler": func(c *Controller, ns, name string) error {
		if c.hpaLister == nil {
			return errNoLister
		}
		_, err := c.hpaLister.HorizontalPodAutoscalers(ns).Get(name)
		return err
	},
	"service": func(c *Controller, ns, name string) error {
		if c.serviceLister == nil {
			return errNoLister
		}
		_, err := c.serviceLister.Services(ns).Get(name)
		return err
	},
	"ingress": func(c *Controller, ns, name string) error {
		if c.ingressLister == nil {
			return errNoLister
		}
		_, err := c.ingressLister.Ingresses(ns).Get(name)
		return err
	},
	"networkpolicy": func(c *Controller, ns, name string) error {
		if c.netpolLister == nil {
			return errNoLister
		}
		_, err := c.netpolLister.NetworkPolicies(ns).Get(name)
		return err
	},
	"poddisruptionbudget": func(c *Controller, ns, name string) error {
		if c.pdbLister == nil {
			return errNoLister
		}
		_, err := c.pdbLister.PodDisruptionBudgets(ns).Get(name)
		return err
	},
	"namespace": func(c *Controller, _, name string) error {
		if c.namespaceLister == nil {
			return errNoLister
		}
		_, err := c.namespaceLister.Get(name)
		return err
	},
	"resourcequota": func(c *Controller, ns, name string) error {
		if c.resourceQuotaLister == nil {
			return errNoLister
		}
		_, err := c.resourceQuotaLister.ResourceQuotas(ns).Get(name)
		return err
	},
	"limitrange": func(c *Controller, ns, name string) error {
		if c.limitRangeLister == nil {
			return errNoLister
		}
		_, err := c.limitRangeLister.LimitRanges(ns).Get(name)
		return err
	},
	"mutatingwebhookconfiguration": func(c *Controller, _, name string) error {
		if c.mwcLister == nil {
			return errNoLister
		}
		_, err := c.mwcLister.Get(name)
		return err
	},
	"validatingwebhookconfiguration": func(
		c *Controller, _, name string,
	) error {
		if c.vwcLister == nil {
			return errNoLister
		}
		_, err := c.vwcLister.Get(name)
		return err
	},
	"secret": func(c *Controller, ns, name string) error {
		if c.secretLister == nil {
			return errNoLister
		}
		_, err := c.secretLister.Secrets(ns).Get(name)
		return err
	},
	"configmap": func(c *Controller, ns, name string) error {
		if c.configMapLister == nil {
			return errNoLister
		}
		_, err := c.configMapLister.ConfigMaps(ns).Get(name)
		return err
	},
	"serviceaccount": func(c *Controller, ns, name string) error {
		if c.serviceAccountLister == nil {
			return errNoLister
		}
		_, err := c.serviceAccountLister.ServiceAccounts(ns).Get(name)
		return err
	},
	"persistentvolume": func(c *Controller, _, name string) error {
		if c.pvLister == nil {
			return errNoLister
		}
		_, err := c.pvLister.Get(name)
		return err
	},
	"storageclass": func(c *Controller, _, name string) error {
		if c.storageClassLister == nil {
			return errNoLister
		}
		_, err := c.storageClassLister.Get(name)
		return err
	},
	"persistentvolumeclaim": func(c *Controller, ns, name string) error {
		if c.pvcLister == nil {
			return errNoLister
		}
		_, err := c.pvcLister.PersistentVolumeClaims(ns).Get(name)
		return err
	},
}

// resourceAliases maps the short forms incidents sometimes carry onto table
// keys.
var resourceAliases = map[string]string{
	"pvc": "persistentvolumeclaim",
	"hpa": "horizontalpodautoscaler",
	"pv":  "persistentvolume",
	"ns":  "namespace",
}

// pluralSuffixes turns the plural spellings used by some detectors back into
// the singular table keys. Detectors are inconsistent about this -- both
// "deployment" and "deployments" reach incidents -- and a lookup miss here
// would silently downgrade the presence check to "cannot answer".
var pluralSuffixes = [][2]string{
	{"ies", "y"},
	{"es", ""},
	{"s", ""},
}

// lookupExists resolves a resource string to its table entry, trying the
// alias and singular spellings before giving up.
func lookupExists(resource string) (existsFn, bool) {
	if fn, ok := resourceExistsTable[resource]; ok {
		return fn, true
	}
	if alias, ok := resourceAliases[resource]; ok {
		fn, found := resourceExistsTable[alias]
		return fn, found
	}
	for _, suffix := range pluralSuffixes {
		if !strings.HasSuffix(resource, suffix[0]) {
			continue
		}
		singular := strings.TrimSuffix(resource, suffix[0]) + suffix[1]
		if fn, ok := resourceExistsTable[singular]; ok {
			return fn, true
		}
	}
	return nil, false
}

// ResourceExists reports whether the object an incident is about is still in
// the informer cache, and whether this controller can answer at all.
//
// The correlation engine closes an incident it has not heard about for a whole
// window. That is right when the object is gone, and wrong when the object is
// still there and simply stopped producing events -- a Deployment stuck on a
// failed rollout writes nothing further, and the incident was resolved while
// the rollout was still stuck. Asking the cache separates the two.
//
// known is false for kinds this controller does not watch, in which case the
// caller must fall back to its own policy rather than assuming absence.
func (c *Controller) ResourceExists(
	resource, namespace, name string,
) (exists, known bool) {
	if name == "" {
		return false, false
	}
	lookup, ok := lookupExists(resource)
	if !ok {
		return false, false
	}
	err := lookup(c, namespace, name)
	switch {
	case err == nil:
		return true, true
	case apierrors.IsNotFound(err):
		return false, true
	default:
		// No lister, or a cache that cannot answer: not evidence of deletion.
		return false, false
	}
}
