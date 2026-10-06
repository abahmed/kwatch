package kube

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// webhookMetricsSource writes the call statistics of admission webhooks
// onto the webhook configurations the informers observe. It only adds to
// them: a name the model does not know is not a configuration.
const webhookMetricsSource = "webhook-metrics"

// Attributes of a webhook configuration, from the API server's metrics
// of its calls since the previous round. The metrics name each webhook
// of a configuration, so these describe the busiest one.
const (
	// AttrWebhookP99 is the 99th percentile of one call, in
	// milliseconds, and AttrWebhookCalls the calls it covers.
	AttrWebhookP99   = "webhook.p99.ms"
	AttrWebhookCalls = "webhook.calls"
	// AttrWebhookSlowest is the webhook of the configuration that is
	// slowest.
	AttrWebhookSlowest = "webhook.slowest"
	// AttrWebhookClosedShare is the percentage of calls that failed and
	// so rejected the request, and AttrWebhookOpenShare the percentage
	// that failed and were ignored.
	AttrWebhookClosedShare = "webhook.failed.closed.percent"
	AttrWebhookOpenShare   = "webhook.failed.open.percent"
)

// minWebhookCalls is how many calls a webhook needs in a round before
// its numbers mean anything.
const minWebhookCalls = 5

// webhookStat is one webhook's calls since the previous round.
type webhookStat struct {
	p99                   float64
	calls, closed, opened float64
}

// webhookObservations turns the webhook metrics of one round into
// observations of the webhook configurations that hold those webhooks.
// Every configuration gets one, empty when it had too few calls, so a
// webhook that stopped being called does not keep its last numbers.
func (p *Prober) webhookObservations(
	now, before *planeReading, at time.Time,
) []inventory.Observation {
	if p.cfg.Model == nil {
		return nil
	}
	var out []inventory.Observation
	for _, kind := range []inventory.Kind{
		KindMutatingWebhook, KindValidatingHook,
	} {
		for _, id := range p.cfg.Model.Entities(kind) {
			entity, ok := p.cfg.Model.Entity(id)
			if !ok {
				continue
			}
			names, _ := entity.Attribute(AttrWebhookNames)
			hooks := strings.Split(names.Value.AsText(), ",")
			out = append(out, inventory.Observation{
				Kind: inventory.Observed, Source: webhookMetricsSource,
				At: at, Entity: id,
				Attributes: webhookAttributes(hooks, now, before),
			})
		}
	}
	return out
}

// webhookAttributes sums the calls of a configuration's webhooks and
// takes the percentile of the slowest one.
func webhookAttributes(
	hooks []string, now, before *planeReading,
) map[string]inventory.Value {
	var total webhookStat
	slowest := ""
	for _, hook := range hooks {
		stat, ok := webhookCalls(hook, now, before)
		if !ok {
			continue
		}
		total.calls += stat.calls
		total.closed += stat.closed
		total.opened += stat.opened
		if stat.p99 > total.p99 {
			total.p99, slowest = stat.p99, hook
		}
	}
	attrs := map[string]inventory.Value{}
	if total.calls < minWebhookCalls {
		return attrs
	}
	attrs[AttrWebhookP99] = inventory.Number(total.p99 * 1000)
	attrs[AttrWebhookCalls] = inventory.Number(total.calls)
	attrs[AttrWebhookSlowest] = inventory.Text(slowest)
	attrs[AttrWebhookClosedShare] = inventory.Number(
		100 * total.closed / total.calls)
	attrs[AttrWebhookOpenShare] = inventory.Number(
		100 * total.opened / total.calls)
	return attrs
}

// webhookCalls is what one webhook did since the previous reading. It is
// false when the webhook has no call or a counter went backwards.
func webhookCalls(
	hook string, now, before *planeReading,
) (webhookStat, bool) {
	calls := now.counters["webhook/calls/"+hook] -
		before.counters["webhook/calls/"+hook]
	closed := now.counters["webhook/closed/"+hook] -
		before.counters["webhook/closed/"+hook]
	opened := now.counters["webhook/open/"+hook] -
		before.counters["webhook/open/"+hook]
	if calls <= 0 || closed < 0 || opened < 0 {
		return webhookStat{}, false
	}
	stat := webhookStat{calls: calls, closed: closed, opened: opened}
	delta, ok := deltaOf(now.webhooks.get(hook), before.webhooks.get(hook))
	if !ok {
		return webhookStat{}, false
	}
	stat.p99, _ = delta.quantile(0.99)
	return stat, true
}
