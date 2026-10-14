package kube

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
)

// serving is a Deployment in kube-system for the provisioner tests. A
// pod is made only when image is set.
type serving struct {
	name, image, driverNames string
}

func provisionerModel(t *testing.T, list ...serving) inventory.Reader {
	t.Helper()
	m := inventory.NewModel(inventory.Options{})
	apply := func(o inventory.Observation) {
		o.Source = "test"
		if _, err := m.Apply(o); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range list {
		dep := inventory.CoreID(KindDeployment, "kube-system", s.name)
		apply(inventory.Observation{Kind: inventory.Observed, Entity: dep,
			Attributes: map[string]inventory.Value{}})
		if s.image == "" {
			continue
		}
		pod := inventory.CoreID(KindPod, "kube-system", s.name+"-1-a")
		img := inventory.CoreID(KindImage, "", s.image)
		attrs := map[string]inventory.Value{}
		if s.driverNames != "" {
			attrs[AttrDriverNames] = inventory.Text(s.driverNames)
		}
		apply(inventory.Observation{Kind: inventory.Observed, Entity: pod,
			Attributes: attrs})
		apply(inventory.Observation{Kind: inventory.Observed, Entity: img,
			Attributes: map[string]inventory.Value{}})
		apply(inventory.Observation{Kind: inventory.Related, Entity: pod,
			Relation: inventory.OwnedBy,
			Targets:  []inventory.EntityID{dep}})
		apply(inventory.Observation{Kind: inventory.Related, Entity: pod,
			Relation: inventory.Pulls,
			Targets:  []inventory.EntityID{img}})
	}
	return m
}

func workloadNames(m inventory.Reader, provisioner string) string {
	var names []string
	for _, id := range ProvisionerWorkloads(m, provisioner) {
		names = append(names, id.Name)
	}
	return strings.Join(names, ",")
}

func TestProvisionerWorkloadsMatch(t *testing.T) {
	m := provisionerModel(t,
		serving{"ebs-csi-controller", "registry.example.com/app:1", ""},
		serving{"storage-ctl",
			"registry.example.com/aws-ebs-csi-driver:1", ""},
		serving{"ebs-csi-node",
			"registry.example.com/aws-ebs-csi-driver:1", ""},
		serving{"dns", "registry.example.com/coredns:1", ""})

	got := workloadNames(m, "ebs.csi.example.com")

	if got != "ebs-csi-controller,storage-ctl" {
		t.Fatalf("got %q", got)
	}
}

func TestProvisionerWorkloadsLookAlikesDoNotMatch(t *testing.T) {
	m := provisionerModel(t,
		serving{"ebs-billing-controller", "shop/billing:1", ""},
		serving{"aws-ebs-csi-controller", "x/aws-ebs-csi-driver:1", ""},
		serving{"kubernetes-csi-snapshot-controller",
			"sig-storage/snapshot-controller:1", ""},
		serving{"csi-azurefile-controller", "mcr/azurefile-csi:1", ""},
		serving{"filebeat-controller", "elastic/filebeat:1", ""},
		serving{"csi-gce-pd-controller",
			"gke.gcr.io/gcp-compute-persistent-disk-csi-driver:1", ""})
	for provisioner, want := range map[string]string{
		"ebs.csi.aws.com":        "aws-ebs-csi-controller",
		"kubernetes.io/aws-ebs":  "",
		"driver.longhorn.io":     "",
		"file.csi.azure.com":     "",
		"csi.vsphere.vmware.com": "",
	} {
		t.Run(provisioner, func(t *testing.T) {
			if got := workloadNames(m, provisioner); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

func TestProvisionerWorkloadsFullNameBeatsTokens(t *testing.T) {
	m := provisionerModel(t,
		serving{"csi-provisioner", "x/csi-provisioner:1", ""},
		serving{"gke-csi-helper", "x/helper:1", ""},
		serving{"pd-ctl", "x/ctl:1", "pd.csi.storage.gke.io"})

	if got := workloadNames(m, "pd.csi.storage.gke.io"); got != "pd-ctl" {
		t.Fatalf("got %q", got)
	}
}

func TestProvisionerWorkloadsScaledToZeroMatchesByName(t *testing.T) {
	m := provisionerModel(t, serving{"ebs-csi-controller", "", ""})

	if got := workloadNames(m, "ebs.csi.aws.com"); got != "ebs-csi-controller" {
		t.Fatalf("got %q", got)
	}
}

func TestProvisionerWorkloadsNodeIsAWholeWord(t *testing.T) {
	m := provisionerModel(t,
		serving{"nodejs-ebs-csi-controller", "", ""},
		serving{"ebs-csi-node-windows", "", ""})

	got := workloadNames(m, "ebs.csi.aws.com")

	if got != "nodejs-ebs-csi-controller" {
		t.Fatalf("got %q", got)
	}
}
