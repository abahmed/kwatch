package dashboard

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func TestDetailContainersReadStatusAndUsage(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	died := testEpoch.Add(-3 * time.Minute)
	tm.container(pod, "api", map[string]inventory.Value{
		kube.AttrReady:         inventory.Bool(false),
		kube.AttrRestarts:      inventory.Number(7),
		kube.AttrState:         inventory.Text("waiting"),
		kube.AttrStateReason:   inventory.Text("CrashLoopBackOff"),
		kube.AttrLastReason:    inventory.Text("OOMKilled"),
		kube.AttrLastExitCode:  inventory.Number(137),
		kube.AttrLastFinished:  inventory.Time(died),
		kube.AttrImage:         inventory.Text("reg.io/api:2"),
		kube.AttrMemoryLimit:   inventory.Number(1 << 30),
		kube.AttrMemoryWorking: inventory.Number(980 << 20),
		kube.AttrCPUUsageMilli: inventory.Number(250),
	})
	tm.container(pod, "proxy", map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(true), kube.AttrRestarts: inventory.Number(0),
	})
	d := detailOf(t, contextServer(tm.m, false,
		failingAt("c", root, testEpoch.Add(-time.Hour))), "c")

	if len(d.Containers) != 2 {
		t.Fatalf("containers = %+v", d.Containers)
	}
	api := d.Containers[0]
	if api.Container != "api" || api.Pod != "api-1" || api.Restarts != 7 ||
		api.Ready == nil || *api.Ready || api.LastReason != "OOMKilled" ||
		api.LastExitCode == nil || *api.LastExitCode != 137 ||
		!api.LastFinished.Equal(died) || api.Image != "reg.io/api:2" ||
		api.StateReason != "CrashLoopBackOff" {
		t.Errorf("api row = %+v", api)
	}
	if api.MemoryUsed != 980<<20 || api.MemoryLimit != 1<<30 ||
		api.CPUUsed != 250 || api.UsageAt.IsZero() {
		t.Errorf("api usage = %+v", api)
	}
	if p := d.Containers[1]; p.MemoryUsed != 0 || !p.UsageAt.IsZero() {
		t.Errorf("proxy has usage: %+v", p)
	}
}

func TestDetailContainersAreBoundedMostRestartedFirst(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	for i := range maxContainers + 2 {
		tm.container(pod, fmt.Sprintf("c%02d", i),
			map[string]inventory.Value{
				kube.AttrRestarts: inventory.Number(float64(i))})
	}
	d := detailOf(t, contextServer(tm.m, false,
		failingAt("c", root, testEpoch)), "c")
	if len(d.Containers) != maxContainers ||
		d.Containers[0].Restarts != maxContainers+1 ||
		d.Truncated == nil || d.Truncated.Containers != 2 {
		t.Errorf("containers %d first %+v truncated %+v",
			len(d.Containers), d.Containers[0], d.Truncated)
	}
}

func TestDetailAffectedListsRootFailingAndImpactWithCommands(
	t *testing.T,
) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	tm.container(pod, "api", map[string]inventory.Value{
		kube.AttrRestarts: inventory.Number(2)})
	since := testEpoch.Add(-20 * time.Minute)
	p := failingAt("c", root, since)
	member := detection.Finding{Entity: kube.ContainerID("shop", "api-1",
		"api"), Reason: "OOMKilled", Since: since.Add(time.Minute)}
	p.Members[member.Key()] = member
	svc := inventory.CoreID(kube.KindService, "shop", "api")
	p.Impact = []inventory.EntityID{svc}

	d := detailOf(t, contextServer(tm.m, false, p), "c")

	if len(d.Affected) != 3 {
		t.Fatalf("affected = %+v", d.Affected)
	}
	want := []string{"deployment/api failing", "pod/api-1 failing",
		"service/api affected"}
	for i, o := range d.Affected {
		if got := o.Kind + "/" + o.Name + " " + o.Status; got != want[i] {
			t.Errorf("affected[%d] = %q, want %q", i, got, want[i])
		}
	}
	if !d.Affected[0].Root || !d.Affected[0].Since.Equal(since) {
		t.Errorf("root = %+v", d.Affected[0])
	}
	cmds := d.Affected[1].Commands
	if len(cmds) != 2 || cmds[0] != "kubectl -n shop describe pod api-1" ||
		cmds[1] != "kubectl -n shop logs api-1 -c api --previous" {
		t.Errorf("pod commands = %q", cmds)
	}
}

func TestDetailAffectedLabelsTheObjectTheCauseBlames(t *testing.T) {
	root := inventory.CoreID(kube.KindDeployment, "shop", "api")
	cm := inventory.CoreID(kube.KindConfigMap, "shop", "app-config")
	pod := kube.ContainerID("shop", "api-1", "api")
	p := failingAt("c", pod, testEpoch)
	p.Root = root
	p.Cause = &rootcause.CauseRecord{Root: root,
		Change: &inventory.Change{Entity: cm, At: testEpoch}}
	d := toDetail(p)
	var got []string
	for _, o := range d.Affected {
		got = append(got, o.Kind+"/"+o.Name+" "+o.Status)
	}
	want := "[deployment/api affected configmap/app-config cause " +
		"pod/api-1 failing]"
	if fmt.Sprint(got) != want {
		t.Errorf("affected = %v, want %s", got, want)
	}
	if !d.Affected[0].Root || d.Affected[1].Root {
		t.Errorf("root flags = %+v", d.Affected)
	}
}

func TestDetailAffectedIsBoundedAndWorksWithoutModel(t *testing.T) {
	root := inventory.CoreID(kube.KindNode, "", "n1")
	p := failingAt("c", root, testEpoch)
	for i := range maxAffected + 4 {
		p.Impact = append(p.Impact, inventory.CoreID(kube.KindPod, "shop",
			fmt.Sprintf("p%02d", i)))
	}
	p.Impact = append(p.Impact, inventory.CoreID(kube.KindZone, "", "z1"))
	d := toDetail(p)
	if len(d.Affected) != maxAffected || d.Truncated == nil ||
		d.Truncated.Affected != 6 {
		t.Fatalf("affected %d truncated %+v", len(d.Affected), d.Truncated)
	}
	if c := d.Affected[0].Commands; len(c) != 1 ||
		c[0] != "kubectl describe node n1" {
		t.Errorf("node commands = %q", c)
	}
}

func TestCommandsForSkipsObjectsKubectlCannotDescribe(t *testing.T) {
	if c := commandsFor(nil, inventory.CoreID(kube.KindImage, "",
		"api:1")); len(c) != 0 {
		t.Errorf("image commands = %q", c)
	}
	cr := inventory.NewEntityID("cert-manager.io", "certificate", "shop",
		"tls")
	if c := commandsFor(nil, cr); len(c) != 1 || c[0] !=
		"kubectl -n shop describe certificate.cert-manager.io tls" {
		t.Errorf("custom resource commands = %q", c)
	}
}
