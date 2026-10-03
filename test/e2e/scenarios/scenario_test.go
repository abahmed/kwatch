//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

// Scenario is one real-cluster test. It owns a fresh namespace that is
// deleted when the test ends, so a scenario only says what it creates, what
// Kwatch must announce and what must happen when the problem is fixed.
//
//	func TestScenarioPodCrashLoop(t *testing.T) {
//		inNamespace(t, "pod.crashloop", func(s *Scenario) {
//			s.Must(createFailingDeployment(s.Ctx, s.Env, s.Namespace, "api"))
//			s.ExpectWorkloadRoot("deployment", "api")
//		})
//	}
//
// Every helper stops the test on failure, so scenario code has no error
// handling and reads from top to bottom.
type Scenario struct {
	T         *testing.T
	Ctx       context.Context
	Env       *harness.Environment
	Namespace string
	started   time.Time
}

// inNamespace runs a scenario in its own namespace. id is the scenario ID
// from test/e2e/coverage/coverage.yaml.
func inNamespace(t *testing.T, id string, run func(s *Scenario)) {
	t.Helper()
	runScenario(t, id, func(
		ctx context.Context, t *testing.T, e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		run(&Scenario{T: t, Ctx: ctx, Env: e, Namespace: namespace,
			started: time.Now()})
	})
}

// onCluster runs a scenario that has no namespace of its own, such as a
// node, webhook or APIService failure.
func onCluster(t *testing.T, id string, run func(s *Scenario)) {
	t.Helper()
	runScenario(t, id, func(
		ctx context.Context, t *testing.T, e *harness.Environment,
	) {
		run(&Scenario{T: t, Ctx: ctx, Env: e, started: time.Now()})
	})
}

// Must stops the test if err is not nil.
func (s *Scenario) Must(err error) {
	s.T.Helper()
	if err != nil {
		s.T.Fatal(err)
	}
}

// ExpectIncident waits until Kwatch announces an incident whose root is
// named resource and whose reasons include reason. sustain is how long the
// detector waits before raising it (0 when it raises at once).
func (s *Scenario) ExpectIncident(
	resource, reason string, sustain time.Duration,
) {
	s.T.Helper()
	s.waitForAudit(announceWait(sustain), harness.AuditMatch{
		Namespace: s.Namespace, Resource: resource,
		Reason: reason, Action: "create", Count: 1,
	})
}

// ExpectResolved waits until that incident is closed. Call it after the
// scenario has fixed the problem.
func (s *Scenario) ExpectResolved(resource, reason string) {
	s.T.Helper()
	s.waitForAudit(resolveWait(), harness.AuditMatch{
		Namespace: s.Namespace, Resource: resource,
		Reason: reason, Action: "resolved", Count: 1,
	})
}

// ExpectClusterIncident is ExpectIncident for a root without a namespace,
// such as a node or an APIService.
func (s *Scenario) ExpectClusterIncident(
	resource, reason string, sustain time.Duration,
) {
	s.T.Helper()
	s.waitForAudit(announceWait(sustain), harness.AuditMatch{
		Resource: resource, Reason: reason, Action: "create", Count: 1,
	})
}

func (s *Scenario) waitForAudit(wait time.Duration, m harness.AuditMatch) {
	s.T.Helper()
	ctx, cancel := context.WithTimeout(s.Ctx, wait)
	defer cancel()
	if _, err := s.Env.Audit.WaitFor(ctx, m); err != nil {
		s.T.Fatalf("waiting for %+v: %v", m, err)
	}
}

// ExpectWorkloadRoot checks that one notification-tier incident is rooted at
// the workload (kind is "deployment", "statefulset", ...), that it did not
// blame the nodes the pods run on, and that it took at most two messages.
func (s *Scenario) ExpectWorkloadRoot(kind, name string) {
	s.T.Helper()
	s.ExpectRoot(harness.RootExpectation{
		Root:         kind + "/" + s.Namespace + "/" + name,
		Tier:         "notify",
		MaxMessages:  2,
		MustNotBlame: scheduledNodes(s.Ctx, s.T, s.Env, s.Namespace),
	})
}

// ExpectRoot is the general form of ExpectWorkloadRoot.
func (s *Scenario) ExpectRoot(exp harness.RootExpectation) {
	s.T.Helper()
	assertRoot(s.Ctx, s.T, s.Env, s.Namespace, s.started, exp)
}

// WaitForPods waits until the namespace's pods satisfy done.
func (s *Scenario) WaitForPods(
	timeout time.Duration, done func(pods []corev1.Pod) bool,
) {
	s.T.Helper()
	ctx, cancel := context.WithTimeout(s.Ctx, timeout)
	defer cancel()
	s.Must(s.Env.WaitForPodCount(ctx, s.Namespace, done))
}

// ExpectKwatchHealthy checks that Kwatch is live and ready and did not
// panic while the scenario ran.
func (s *Scenario) ExpectKwatchHealthy() {
	s.T.Helper()
	s.Must(s.Env.Health.AssertOK(s.Ctx, "/healthz"))
	s.Must(s.Env.Health.AssertOK(s.Ctx, "/readyz"))
	s.Must(s.Env.AssertNoRuntimePanic(s.Ctx))
}
