package resource

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

func TestOvercommitLevelClassifiesWarningAndCritical(t *testing.T) {
	cfg := Config{
		CpuWarning: 0.7, CpuCritical: 0.9,
		MemWarning: 0.7, MemCritical: 0.9,
	}

	tests := []struct {
		name        string
		cpu, memory float64
		wantReason  string
		wantLevel   string
	}{
		{name: "healthy", cpu: 0.4, memory: 0.4},
		{
			name: "warning", cpu: 0.8, memory: 0.4,
			wantReason: constant.ReasonNodeResourceHigh,
			wantLevel:  "warning",
		},
		{
			name: "critical", cpu: 0.95, memory: 0.4,
			wantReason: constant.ReasonNodeResourceCritical,
			wantLevel:  "critical",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reason, hint, severity := overcommitLevel(
				"node-a", test.cpu, test.memory, cfg,
			)
			if reason != test.wantReason {
				t.Fatalf("reason = %q, want %q", reason, test.wantReason)
			}
			if test.wantReason != "" &&
				(string(severity) != test.wantLevel || hint == "") {
				t.Fatalf("unexpected level or hint: %q %q", severity, hint)
			}
		})
	}
}

func TestFilesystemSignalsClassifiesFilesystemAndInodes(t *testing.T) {
	capacity, used := uint64(100), uint64(85)
	inodes, free := uint64(1000), uint64(50)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}
	cfg := Config{
		FilesystemWarningPercent: 80, FilesystemCriticalPercent: 90,
		InodeWarningPercent: 80, InodeCriticalPercent: 95,
	}
	signals := filesystemSignals(node, &filesystemStats{
		CapacityBytes: &capacity, UsedBytes: &used,
		Inodes: &inodes, InodesFree: &free,
	}, cfg)
	if len(signals) != 2 {
		t.Fatalf("signals = %d, want 2", len(signals))
	}
	if signals[0].Reason != constant.ReasonNodeFilesystemHigh {
		t.Fatalf("filesystem reason = %q", signals[0].Reason)
	}
	if signals[1].Reason != constant.ReasonNodeInodesCritical {
		t.Fatalf("inode reason = %q", signals[1].Reason)
	}
}

func TestThresholdSignalIgnoresDisabledAndBelowThreshold(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}
	if got := thresholdSignal(
		node, 90, 0, 95, "warning", "critical", "filesystem",
	); got != nil {
		t.Fatal("disabled warning threshold produced a signal")
	}
	if got := thresholdSignal(
		node, 79, 80, 95, "warning", "critical", "filesystem",
	); got != nil {
		t.Fatal("below-threshold usage produced a signal")
	}
}

func TestFilesystemSignalsSkipsInvalidCounters(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}
	zero := uint64(0)
	used := uint64(10)
	if got := filesystemSignals(node, &filesystemStats{
		CapacityBytes: &zero, UsedBytes: &used,
	}, Config{FilesystemWarningPercent: 80}); len(got) != 0 {
		t.Fatalf("zero capacity produced signals: %+v", got)
	}
	if got := filesystemSignals(node, &filesystemStats{}, Config{
		InodeWarningPercent: 80,
	}); len(got) != 0 {
		t.Fatalf("missing inode counters produced signals: %+v", got)
	}
}

func TestOvercommitLevelConsidersMemoryRatio(t *testing.T) {
	reason, _, severity := overcommitLevel(
		"node-a", 0.1, 0.8,
		Config{
			CpuWarning: 0.7, CpuCritical: 0.95,
			MemWarning: 0.7, MemCritical: 0.95,
		},
	)
	if reason != constant.ReasonNodeResourceHigh ||
		severity != model.SeverityWarning {
		t.Fatalf("memory overcommit = %q, %q", reason, severity)
	}
}

func TestCheckSkipsTerminalPodsAndReportsOvercommit(t *testing.T) {
	nodeIndex := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	podIndex := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		}},
	}
	running := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "running", Namespace: "apps"},
		Spec: corev1.PodSpec{
			NodeName: "node-a",
			Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("900m"),
				},
			}}},
		},
	}
	terminal := running.DeepCopy()
	terminal.Name = "completed"
	terminal.Status.Phase = corev1.PodSucceeded
	if err := nodeIndex.Add(node); err != nil {
		t.Fatal(err)
	}
	if err := podIndex.Add(running); err != nil {
		t.Fatal(err)
	}
	if err := podIndex.Add(terminal); err != nil {
		t.Fatal(err)
	}
	monitor := &Monitor{
		cfg: Config{
			CpuWarning: 0.8, CpuCritical: 0.95,
			MemWarning: 2, MemCritical: 2,
		},
		nodeLister: corev1listers.NewNodeLister(nodeIndex),
		podLister:  corev1listers.NewPodLister(podIndex),
	}
	signals := monitor.Check()
	if len(signals) != 1 || signals[0].Reason != constant.ReasonNodeResourceHigh {
		if len(signals) == 1 {
			t.Fatalf("unexpected overcommit reason: %q", signals[0].Reason)
		}
		t.Fatalf("unexpected overcommit signals: %+v", signals)
	}
}
