package dashboard

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// fakeUsage answers n hourly peaks for every container it knows.
func fakeUsage(n int, known ...inventory.EntityID) usageSource {
	return func(id inventory.EntityID) []kube.UsageHour {
		for _, k := range known {
			if k != id {
				continue
			}
			out := make([]kube.UsageHour, 0, n)
			for i := range n {
				out = append(out, kube.UsageHour{
					Start:       testEpoch.Add(time.Duration(i-n) * time.Hour),
					MemoryBytes: float64(i+1) * (1 << 20),
					CPUMilli:    float64(i + 1)})
			}
			return out
		}
		return nil
	}
}

func usageServer(
	m inventory.HistoryReader, src usageSource, p incident.Incident,
) *Server {
	s := contextServer(m, false, p)
	s.SetUsageHours(src)
	return s
}

func TestDetailUsageHoursForAnOOMKilledContainer(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	api := tm.container(pod, "api", map[string]inventory.Value{
		kube.AttrLastReason:  inventory.Text("OOMKilled"),
		kube.AttrMemoryLimit: inventory.Number(64 << 20),
	})
	proxy := tm.container(pod, "proxy", nil)
	p := failingAt("a", root, testEpoch)
	d := detailOf(t, usageServer(tm.m, fakeUsage(30, api, proxy), p), "a")
	if len(d.UsageHours) != 1 {
		t.Fatalf("usageHours = %+v", d.UsageHours)
	}
	h := d.UsageHours[0]
	if h.Container != "api" || h.Pod != "api-1" || h.Namespace != "shop" ||
		h.Metric != metricMemory || h.Limit != 64<<20 ||
		len(h.Peaks) != maxUsagePeaks || h.Peaks[23].Value != 30<<20 {
		t.Errorf("history = %+v", h)
	}
}

func TestDetailUsageHoursAbsentForOtherIncidents(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	api := tm.container(pod, "api", map[string]inventory.Value{
		kube.AttrMemoryLimit:   inventory.Number(1 << 30),
		kube.AttrMemoryWorking: inventory.Number(100 << 20),
	})
	p := failingAt("a", root, testEpoch)
	d := detailOf(t, usageServer(tm.m, fakeUsage(5, api), p), "a")
	if d.UsageHours != nil {
		t.Errorf("usageHours = %+v", d.UsageHours)
	}
	p.Mode = detection.ModeOOMKilledLimit
	d = detailOf(t, contextServer(tm.m, false, p), "a")
	if d.UsageHours != nil {
		t.Errorf("without a source: %+v", d.UsageHours)
	}
	d = detailOf(t, usageServer(tm.m, fakeUsage(0, api), p), "a")
	if d.UsageHours != nil {
		t.Errorf("without readings: %+v", d.UsageHours)
	}
}

func TestDetailUsageHoursCPUOnlyForThrottling(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	api := tm.container(pod, "api", map[string]inventory.Value{
		kube.AttrCPULimit: inventory.Number(500)})
	p := failingWith("a", root, reasons.ContainerCPUThrottled, api)
	d := detailOf(t, usageServer(tm.m, fakeUsage(3, api), p), "a")
	if len(d.UsageHours) != 1 || d.UsageHours[0].Metric != metricCPU ||
		d.UsageHours[0].Limit != 500 || d.UsageHours[0].Peaks[2].Value != 3 {
		t.Errorf("usageHours = %+v", d.UsageHours)
	}
}

func TestDetailUsageHoursAreBounded(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	var ids []inventory.EntityID
	for i := range 5 {
		ids = append(ids, tm.container(pod, fmt.Sprintf("c%d", i), nil))
	}
	p := failingWith("a", root, reasons.OOMKilled, root)
	d := detailOf(t, usageServer(tm.m, fakeUsage(2, ids...), p), "a")
	if len(d.UsageHours) != maxUsageHistories || d.Truncated == nil ||
		d.Truncated.UsageHours != 2 {
		t.Errorf("usage %d truncated %+v", len(d.UsageHours), d.Truncated)
	}
}

func TestServerSetUsageHoursNilRemovesTheSource(t *testing.T) {
	s := newTestServer()
	s.SetUsageHours(fakeUsage(1))
	if s.sources.usage.Load() == nil {
		t.Fatal("source not set")
	}
	s.SetUsageHours(nil)
	if s.sources.usage.Load() != nil {
		t.Error("source not removed")
	}
}
