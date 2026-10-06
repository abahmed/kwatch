package detectors

import (
	"fmt"
	"math"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// etcdSize reports an etcd database near its default quota. etcd stops
// accepting writes at the quota, which stops the whole cluster, so the
// approach is worth a line even when a managed control plane allows more.
func etcdSize(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	size, ok := number(e, kube.AttrEtcdDBBytes)
	limit := etcdNearShare * etcdQuotaBytes
	if !ok || !held(ctx, "etcd-large", size, limit) {
		return nil
	}
	since := ctx.Onset("etcd-large", ctx.Now)
	if !sustained(ctx, "etcd-large", since, DefaultProbeFailing) {
		return nil
	}
	return []detection.Finding{{
		Reason: reasons.EtcdDatabaseLarge, Severity: detection.Info,
		Since: since,
		Summary: "etcd database is " + gibText(size) +
			", close to its default 2 GiB limit",
		Evidence: []detection.Evidence{
			{Label: "most objects", Value: objectsText(e)},
			{Label: "what it means", Value: "At the limit etcd " +
				"refuses writes; compact and defragment it, or " +
				"remove what fills it"},
		},
	}}
}

// manyStored reports one resource with a very large number of objects:
// every list of it is a heavy call, and etcd grows with it.
func manyStored(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	count, ok := number(e, kube.AttrObjectsCount)
	if !ok || !held(ctx, "many-objects", count, manyObjects) {
		return nil
	}
	since := ctx.Onset("many-objects", ctx.Now)
	if !sustained(ctx, "many-objects", since, DefaultProbeFailing) {
		return nil
	}
	resource := text(e, kube.AttrObjectsResource)
	return []detection.Finding{{
		Reason: reasons.StorageObjectsHigh, Severity: detection.Info,
		Since: since,
		Summary: fmt.Sprintf("etcd stores %s %s objects",
			thousandsText(count), resource),
		Evidence: []detection.Evidence{
			{Label: "resource", Value: resource},
			{Label: "what it means", Value: "Lists of it are heavy " +
				"calls and the database grows with it; find what " +
				"creates them and clean them up"},
		},
	}}
}

func objectsText(e inventory.Entity) string {
	count, ok := number(e, kube.AttrObjectsCount)
	if !ok {
		return "unknown"
	}
	return thousandsText(count) + " " + text(e, kube.AttrObjectsResource)
}

// gibText writes bytes as GiB with one decimal.
func gibText(bytes float64) string {
	return fmt.Sprintf("%.1f GiB", bytes/(1<<30))
}

// thousandsText writes a count with thousands separators: "240,000".
func thousandsText(n float64) string {
	digits := fmt.Sprintf("%.0f", n)
	out := make([]byte, 0, len(digits)+len(digits)/3)
	for i := range len(digits) {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, digits[i])
	}
	return string(out)
}

// millisText writes milliseconds as "850 ms" or "2.4 seconds". It never
// writes "2.4s": notes rewrite that as a rough, minute-sized time.
func millisText(ms float64) string {
	if ms < 1000 {
		return fmt.Sprintf("%.0f ms", ms)
	}
	seconds := ms / 1000
	if seconds == math.Trunc(seconds) {
		return wholeSeconds(seconds)
	}
	return fmt.Sprintf("%.1f seconds", seconds)
}

func wholeSeconds(s float64) string {
	if s == 1 {
		return "1 second"
	}
	return fmt.Sprintf("%.0f seconds", s)
}
