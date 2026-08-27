package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestProcessPodListerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.Pod = &errorPodLister{f.Core().V1().Pods().Lister()}
	assert.Error(t, h.ProcessPod(context.Background(), "ns/p1", false))
}

func TestProcessNodeListerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.Node = &errorNodeLister{f.Core().V1().Nodes().Lister()}
	assert.Error(t, h.ProcessNode("test-node", false))
}

func TestProcessDeploymentListerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.Deploy = &errorDeployLister{f.Apps().V1().Deployments().Lister()}
	assert.Error(t, h.ProcessDeployment("ns/my-deploy", false))
}

func TestProcessDaemonSetListerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.DS = &errorDSLister{f.Apps().V1().DaemonSets().Lister()}
	assert.Error(t, h.ProcessDaemonSet("ns/my-ds", false))
}

func TestProcessStatefulSetListerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.SS = &errorSSLister{f.Apps().V1().StatefulSets().Lister()}
	assert.Error(t, h.ProcessStatefulSet("ns/my-ss", false))
}

func TestProcessPdbListerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.PDB = &errorPdbLister{
		f.Policy().V1().PodDisruptionBudgets().Lister(),
	}
	assert.Error(t, h.ProcessPdb("ns/my-pdb", false))
}

func TestProcessJobListerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.Job = &errorJobLister{f.Batch().V1().Jobs().Lister()}
	assert.Error(t, h.ProcessJob("ns/my-job", false))
}

func TestProcessCronJobListerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.CronJob = &errorCJLister{f.Batch().V1().CronJobs().Lister()}
	assert.Error(t, h.ProcessCronJob("ns/my-cj", false))
}

func TestProcessHorizontalPodAutoscalerListerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.HPA = &errorHPALister{
		f.Autoscaling().V2().HorizontalPodAutoscalers().Lister(),
	}
	assert.Error(t, h.ProcessHorizontalPodAutoscaler("ns/my-hpa", false))
}

func TestProcessServiceListerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.Service = &errorServiceLister{f.Core().V1().Services().Lister()}
	assert.Error(t, h.ProcessService("ns/my-svc", false))
}

func TestProcessIngressListerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.Ingress = &errorIngressLister{
		f.Networking().V1().Ingresses().Lister(),
	}
	assert.Error(t, h.ProcessIngress("ns/my-ing", false))
}

func TestProcessNetworkPolicyListerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.Netpol = &errorNetpolLister{
		f.Networking().V1().NetworkPolicies().Lister(),
	}
	assert.Error(t, h.ProcessNetworkPolicy("ns/my-netpol", false))
}

func TestProcessMutatingWebhookConfigurationListerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.MWC = &errorMwCLister{
		admv1(f).MutatingWebhookConfigurations().Lister(),
	}
	assert.Error(t, h.ProcessMutatingWebhookConfiguration("test-mwc", false))
}

func TestProcessValidatingWebhookConfigurationListerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.VWC = &errorVwCLister{
		admv1(f).ValidatingWebhookConfigurations().Lister(),
	}
	assert.Error(t, h.ProcessValidatingWebhookConfiguration("test-vwc", false))
}

func TestSweepControlPlaneListerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.CPPod = &errorCpPodLister{f.Core().V1().Pods().Lister()}
	h.SweepControlPlane() // should not panic
}

func TestSweepTLSSecretsListerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.Secret = &errorSecretLister{f.Core().V1().Secrets().Lister()}
	h.SweepTLSSecrets() // should not panic
}
