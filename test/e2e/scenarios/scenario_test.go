//go:build e2e

package scenarios

import (
	"context"
	"os"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

// Scenario is one real-cluster test. It owns a fresh namespace that is
// deleted when the test ends, so a scenario only says what it creates, what
// Kwatch must announce and what must happen when the problem is fixed.
//
//	func TestScenarioPodCrashLoop(t *testing.T) {
//		inNamespace(t, "pod.crashloop", func(s *Scenario) {
//			s.CreateDeployment("api", "crash")
//			s.ExpectWorkloadRoot("deployment", "api")
//		})
//	}
//
// Every helper stops the test on failure, so scenario code has no error
// handling and reads from top to bottom. The helpers that create things
// live in the *_build_test.go files, named after what they build.
type Scenario struct {
	T         *testing.T
	Ctx       context.Context
	Env       *harness.Environment
	Namespace string
	started   time.Time
}

// inNamespace runs a scenario in its own namespace, at the same time as the
// other inNamespace scenarios. Use it when everything the scenario creates
// lives in that namespace. id is the scenario ID from
// test/e2e/coverage/coverage.yaml.
func inNamespace(t *testing.T, id string, run func(s *Scenario)) {
	t.Helper()
	t.Parallel()
	inOwnNamespace(t, id, run)
}

// inNamespaceAlone runs a scenario in its own namespace while no other
// scenario runs. Use it when the scenario disturbs something every scenario
// shares: a node, the Kwatch Pod, the webhook receiver, or the cluster's
// APIs and admission webhooks.
func inNamespaceAlone(t *testing.T, id string, run func(s *Scenario)) {
	t.Helper()
	inOwnNamespace(t, id, run)
}

func inOwnNamespace(t *testing.T, id string, run func(s *Scenario)) {
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
// node, webhook or APIService failure. It runs while no other scenario
// runs.
func onCluster(t *testing.T, id string, run func(s *Scenario)) {
	t.Helper()
	runScenario(t, id, func(
		ctx context.Context, t *testing.T, e *harness.Environment,
	) {
		run(&Scenario{T: t, Ctx: ctx, Env: e, started: time.Now()})
	})
}

// inExtendedNamespace is inNamespace for the extended scenarios, which
// only run when KWATCH_EXTENDED=true.
func inExtendedNamespace(t *testing.T, id string, run func(s *Scenario)) {
	t.Helper()
	skipUnlessExtended(t)
	inNamespace(t, id, run)
}

// inExtendedNamespaceAlone is inNamespaceAlone for the extended scenarios.
func inExtendedNamespaceAlone(
	t *testing.T, id string, run func(s *Scenario),
) {
	t.Helper()
	skipUnlessExtended(t)
	inNamespaceAlone(t, id, run)
}

// inExtendedCluster is onCluster for the extended scenarios.
func inExtendedCluster(t *testing.T, id string, run func(s *Scenario)) {
	t.Helper()
	skipUnlessExtended(t)
	onCluster(t, id, run)
}

func skipUnlessExtended(t *testing.T) {
	t.Helper()
	if os.Getenv("KWATCH_EXTENDED") != "true" {
		t.Skip("set KWATCH_EXTENDED=true for extended Kind scenarios")
	}
}

// knownGap skips a scenario that fails because of a product problem that is
// tracked elsewhere, so the suite stays green without hiding the problem.
// Remove the call once the problem is fixed.
func knownGap(t *testing.T, problem string) {
	t.Helper()
	t.Skip("known gap: " + problem)
}

// Must stops the test if err is not nil.
func (s *Scenario) Must(err error) {
	s.T.Helper()
	if err != nil {
		s.T.Fatal(err)
	}
}

// ExpectIncident waits until Kwatch announces an incident whose root is
// named resource and whose reasons include reason, and returns the incident
// ID for ExpectResolved. The reason may arrive in the first message or in a
// later update of the same incident, so any announcement counts. sustain is
// how long the detector waits before raising it (0 when it raises at once).
func (s *Scenario) ExpectIncident(
	resource, reason string, sustain time.Duration,
) (incident string) {
	s.T.Helper()
	return s.waitForAudit(announceWait(sustain), harness.AuditMatch{
		Namespace: s.Namespace, Resource: resource,
		Reason: reason, Count: 1,
	})
}

// ExpectIncidents waits until Kwatch has announced count incidents for
// resource, for example after a problem came back.
func (s *Scenario) ExpectIncidents(resource, reason string, count int) {
	s.T.Helper()
	s.waitForAudit(announceWait(0), harness.AuditMatch{
		Namespace: s.Namespace, Resource: resource,
		Reason: reason, Action: "create", Count: count,
	})
}

// ExpectClusterIncident is ExpectIncident for a root without a namespace,
// such as a node or an APIService.
func (s *Scenario) ExpectClusterIncident(
	resource, reason string, sustain time.Duration,
) (incident string) {
	s.T.Helper()
	return s.waitForAudit(announceWait(sustain), harness.AuditMatch{
		Resource: resource, Reason: reason, Count: 1,
	})
}

// ExpectResolved waits until the incident returned by ExpectIncident is
// closed. Call it after the scenario has fixed the problem. The incident is
// matched by ID because its root can change while it is open, for example
// from a missing ConfigMap to the Deployment that needed it.
func (s *Scenario) ExpectResolved(incident string) {
	s.T.Helper()
	s.waitForAudit(resolveWait(), harness.AuditMatch{
		Incident: incident, Action: "resolved", Count: 1,
	})
}

// waitForAudit waits for audit entries matching m and returns the incident
// ID of the first one.
func (s *Scenario) waitForAudit(
	timeout time.Duration, m harness.AuditMatch,
) string {
	s.T.Helper()
	ctx, cancel := context.WithTimeout(s.Ctx, timeout)
	defer cancel()
	entries, err := s.Env.Audit.WaitFor(ctx, m)
	if err != nil {
		s.T.Fatalf("waiting for %+v: %v", m, err)
	}
	return entries[0].Incident
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
		MustNotBlame: s.ScheduledNodes(),
	})
}

// ExpectRoot is the general form of ExpectWorkloadRoot. It waits for the
// incident with the expected root, then requires the tier, the message
// budget and the absence of blamed entities. Only audit entries from this
// scenario count.
func (s *Scenario) ExpectRoot(exp harness.RootExpectation) {
	s.T.Helper()
	ctx, cancel := context.WithTimeout(s.Ctx, 8*time.Minute)
	defer cancel()
	scope := harness.RootScope{Namespace: s.Namespace, Since: s.started}
	s.Must(s.Env.Audit.AssertRoot(ctx, exp, scope))
}

// ScheduledNodes lists the nodes hosting the namespace's Pods as
// must-not-blame roots: a workload failure is not a node failure.
func (s *Scenario) ScheduledNodes() []string {
	s.T.Helper()
	pods, err := s.Env.Client.CoreV1().Pods(s.Namespace).List(
		s.Ctx, metav1.ListOptions{},
	)
	s.Must(err)
	seen := make(map[string]bool)
	var roots []string
	for _, pod := range pods.Items {
		if pod.Spec.NodeName != "" && !seen[pod.Spec.NodeName] {
			seen[pod.Spec.NodeName] = true
			roots = append(roots, "node//"+pod.Spec.NodeName)
		}
	}
	return roots
}

// ExpectKwatchHealthy checks that Kwatch becomes live and ready within
// slackTime (a new leader needs a moment to sync) and did not panic while
// the scenario ran.
func (s *Scenario) ExpectKwatchHealthy() {
	s.T.Helper()
	ctx, cancel := context.WithTimeout(s.Ctx, slackTime)
	defer cancel()
	s.Must(wait.PollUntilContextCancel(ctx, 5*time.Second, true,
		func(ctx context.Context) (bool, error) {
			return s.Env.AssertHealthy(ctx) == nil, nil
		}))
	s.Must(s.Env.AssertNoRuntimePanic(s.Ctx))
}
