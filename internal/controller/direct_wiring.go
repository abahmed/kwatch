package controller

import (
	"github.com/abahmed/kwatch/internal/controlplane"
	"github.com/abahmed/kwatch/internal/metrics"
	clustermonitor "github.com/abahmed/kwatch/internal/monitor/cluster"
	networkmonitor "github.com/abahmed/kwatch/internal/monitor/network"
	nodemonitor "github.com/abahmed/kwatch/internal/monitor/node"
	podmonitor "github.com/abahmed/kwatch/internal/monitor/pod"
	securitymonitor "github.com/abahmed/kwatch/internal/monitor/security"
)

// configureDirectRuntimes connects family-owned processors to controller
// caches. Keeping this out of New makes startup wiring read as a composition
// script while keeping every dependency explicit.
func configureDirectRuntimes(
	c *Controller,
	components RuntimeSet,
) error {
	if components.IncidentSources != nil {
		if err := configureIncidentSources(
			c, components.IncidentSources,
		); err != nil {
			return err
		}
	}
	if err := configureDirectWorkloadRuntimes(c, components); err != nil {
		return err
	}
	if components.Pod.Config != nil {
		if err := components.Pod.Config.ConfigureSources(
			podmonitor.RuntimeSources{
				Pod: c.podLister, RS: c.rsLister, DS: c.dsLister,
				SS: c.ssLister, Events: c.eventLister,
				EventsByPod: c.eventsByPod, Secret: c.secretLister,
				ConfigMap:      c.configMapLister,
				ServiceAccount: c.serviceAccountLister,
			},
		); err != nil {
			return err
		}
	}
	if components.Node.Config != nil {
		if err := components.Node.Config.ConfigureSources(
			nodemonitor.Sources{Nodes: c.nodeLister},
		); err != nil {
			return err
		}
	}
	if components.Network.Config != nil {
		if err := components.Network.Config.ConfigureSources(
			networkmonitor.Sources{
				Services:      c.serviceLister,
				EndpointSlice: c.endpointSliceLister,
				Ingresses:     c.ingressLister,
				NetworkPolicy: c.netpolLister,
			},
		); err != nil {
			return err
		}
	}
	if components.Security.Config != nil {
		if err := components.Security.Config.ConfigureSources(
			securitymonitor.Sources{
				MutatingWebhooks:   c.mwcLister,
				ValidatingWebhooks: c.vwcLister,
				Services:           c.serviceLister,
				EndpointSlices:     c.endpointSliceLister,
			},
		); err != nil {
			return err
		}
	}
	if components.Cluster.Config != nil {
		if err := components.Cluster.Config.ConfigureSources(
			clustermonitor.Sources{
				ResourceQuotas:   c.resourceQuotaLister,
				LimitRanges:      c.limitRangeLister,
				Namespaces:       c.namespaceLister,
				Leases:           c.leaseLister,
				NamespaceAllowed: c.NamespaceAllowed,
			},
		); err != nil {
			return err
		}
	}
	if components.Integration.ControlPlaneConfig != nil {
		if err := components.Integration.ControlPlaneConfig.ConfigureSources(
			controlplane.Sources{PodLister: c.cpPodLister},
		); err != nil {
			return err
		}
	}
	if components.Integration.TLSConfig != nil {
		if err := components.Integration.TLSConfig.ConfigureSources(
			securitymonitor.TLSSources{Secrets: c.secretLister},
		); err != nil {
			return err
		}
	}
	c.recordSourceUnavailableTransitions()
	return nil
}

// recordSourceUnavailableTransitions counts each enabled source becoming
// unavailable once. Repeated diagnostics must not inflate the counter.
func (c *Controller) recordSourceUnavailableTransitions() {
	current := make(map[string]bool)
	for _, source := range c.unavailableSources() {
		current[source] = true
	}
	c.informerMu.Lock()
	previous := c.sourceUnavailable
	if previous == nil {
		previous = make(map[string]bool)
	}
	transitions := 0
	for source := range current {
		if !previous[source] {
			transitions++
		}
	}
	c.sourceUnavailable = current
	c.informerMu.Unlock()
	if transitions > 0 {
		metrics.DefaultRegistry().SourceUnavailable.Add(
			int64(transitions),
		)
	}
}

func configureIncidentSources(
	c *Controller,
	sources IncidentSourceConfig,
) error {
	return sources.ConfigureAttributionSources(c.IncidentAttributionSources())
}
