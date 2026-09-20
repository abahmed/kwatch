package controller

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	controlplanemonitor "github.com/abahmed/kwatch/internal/controlplane"
	"github.com/abahmed/kwatch/internal/monitor/network"
	"github.com/abahmed/kwatch/internal/monitor/security"
)

// hasService is a helper seeded by controllers that reference a service.
func (c *Controller) hasService() func(string, string) (bool, error) {
	return func(ns, name string) (bool, error) {
		if c.serviceLister == nil {
			return true, nil
		}
		_, err := c.serviceLister.Services(ns).Get(name)
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	}
}

// seedControllersWithSvc seeds MWC/VWC and NetworkPolicies
func (c *Controller) seedControllersWithSvc(rec *baselineRecorder) {
	hasSvc := c.hasService()
	c.seedMwcs(rec, hasSvc)
	c.seedVwcs(rec, hasSvc)
	c.seedIngresses(rec, hasSvc)
}

func (c *Controller) seedMwcs(
	rec *baselineRecorder,
	hasSvc func(string, string) (bool, error),
) {
	// Admission webhooks — seed webhook-backend issues
	if c.mwcLister != nil {
		mwcs, err := c.mwcLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(
				err,
				"failed to list mutating webhook configurations for baseline "+
					"seeding",
			)
		} else {
			for _, mwc := range mwcs {
				sigs, err := security.DetectMutatingWebhookIssueWithLookup(
					mwc, hasSvc,
				)
				if err != nil {
					klog.ErrorS(err, "skipping mutating webhook baseline")
					continue
				}
				endpointSigs, err := security.DetectWebhookEndpointIssuesWithError(
					c.endpointSliceLister, mwc.Name, mwc.Namespace,
					mwc.Labels, security.MutatingWebhookServices(mwc),
				)
				if err != nil {
					klog.ErrorS(err, "skipping mutating webhook endpoint baseline")
					continue
				}
				sigs = append(sigs, endpointSigs...)
				for _, sig := range sigs {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedVwcs(
	rec *baselineRecorder,
	hasSvc func(string, string) (bool, error),
) {
	if c.vwcLister != nil {
		vwcs, err := c.vwcLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(
				err,
				"failed to list validating webhook configurations for "+
					"baseline seeding",
			)
		} else {
			for _, vwc := range vwcs {
				sigs, err := security.DetectValidatingWebhookIssueWithLookup(
					vwc, hasSvc,
				)
				if err != nil {
					klog.ErrorS(err, "skipping validating webhook baseline")
					continue
				}
				endpointSigs, err := security.DetectWebhookEndpointIssuesWithError(
					c.endpointSliceLister, vwc.Name, vwc.Namespace,
					vwc.Labels, security.ValidatingWebhookServices(vwc),
				)
				if err != nil {
					klog.ErrorS(err, "skipping validating webhook endpoint baseline")
					continue
				}
				sigs = append(sigs, endpointSigs...)
				for _, sig := range sigs {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedIngresses(
	rec *baselineRecorder,
	hasSvc func(string, string) (bool, error),
) {
	// Ingresses — seed ingress-backend issues
	if c.ingressLister != nil {
		ings, err := c.ingressLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list ingresses for baseline seeding")
		} else {
			for _, ing := range ings {
				sigs, err := network.DetectIngressIssueWithLookup(ing, hasSvc)
				if err != nil {
					klog.ErrorS(err, "skipping ingress baseline",
						"namespace", ing.Namespace, "name", ing.Name)
					continue
				}
				for _, sig := range sigs {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedNetworkPolicies(rec *baselineRecorder) {
	// NetworkPolicies — seed restrictive-policy issues
	if c.netpolLister != nil {
		policies, err := c.netpolLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(
				err,
				"failed to list network policies for baseline seeding",
			)
		} else {
			for _, policy := range policies {
				if sig := network.DetectNetworkPolicyIssue(policy); sig != nil {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedControlPlaneBaseline(rec *baselineRecorder) {
	// Control-plane — seed CP component failures. Unlike other owner-level
	// signals, CP signals carry PodName, so we seed with the actual pod name.
	if c.cpPod.startWorkers && c.cpPodLister != nil {
		pods, err := c.cpPodLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(
				err,
				"failed to list control-plane pods for baseline seeding",
			)
		} else {
			for _, pod := range pods {
				if sig := controlplanemonitor.DetectPodIssue(pod); sig != nil {
					rec.seedControlPlane(pod, sig)
				}
			}
		}
	}
}
