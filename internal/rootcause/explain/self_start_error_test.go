package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const configLine = "Unhandled exception. Bus.ConfigurationException: " +
	"The bus configuration is invalid:"

// startFails makes the given pods' containers stop at start with line:
// the run ended with exit code, after one restart or more.
func startFails(
	f *fixture, pods []inventory.EntityID, line string, code float64,
) {
	for _, pod := range pods {
		id := containerOf(pod)
		f.observe(id, map[string]inventory.Value{
			kube.AttrLastErrorLine: inventory.Text(line),
			kube.AttrLastExitCode:  inventory.Number(code),
			kube.AttrRestarts:      inventory.Number(4),
		})
	}
}

// probeRows are the rows a fatal line at start rules out.
var probeRows = map[string]bool{"startup-budget-too-short": true,
	"liveness-shorter-than-start": true,
	"liveness-kills-before-ready": true}

// A liveness kill (exit 137) of a program that already stopped with a
// fatal error at start is a symptom: no probe budget is blamed, however
// long the workload's ready pods took.
func TestStartupErrorRulesOutTheProbeBudgets(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("shop", "api", 3)
	for _, pod := range pods[:2] {
		f.observe(pod, map[string]inventory.Value{
			kube.AttrReady:      inventory.Bool(true),
			kube.AttrStartTime:  inventory.Time(t0.Add(-time.Hour)),
			kube.AttrReadySince: inventory.Time(t0.Add(-20 * time.Minute)),
		})
	}
	failing := pods[2:]
	f.observe(containerOf(failing[0]), map[string]inventory.Value{
		kube.AttrProbeBudget: inventory.Number(80),
	})
	startFails(f, failing, configLine, 137)
	liveKill(f, failing, livenessGives, neverReady)
	f.fail(containerOf(failing[0]), "Probe.Startup", failingH, 2, "")
	if c, ok := f.explain().CauseOf(containerOf(failing[0])); ok &&
		probeRows[c.Row] {
		t.Fatalf("blamed %s after a fatal line at start", c.Row)
	}
}

// A new template that ships a build that stops at start is the cause on
// every solve, and the record quotes the line it stops with.
func TestStartupErrorKeepsTheReleaseAsTrigger(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("shop", "api", 2)
	deployment := inventory.CoreID(kube.KindDeployment, "shop", "api")
	f.change(deployment, 1, "spec.template.spec.containers[0].image")
	startFails(f, pods, configLine, 137)
	for _, pod := range pods {
		f.fail(containerOf(pod), "CrashLoop", failingH, 2, "")
	}
	for i := 0; i < 3; i++ {
		c := requireCause(t, f.explain(), containerOf(pods[0]),
			"deployment/shop/api")
		if c.Row != "rollout" {
			t.Fatalf("solve %d: row = %s", i, c.Row)
		}
		rec := f.snapshot().Record(c)
		if rec.Change == nil || rec.SharedError != configLine {
			t.Fatalf("solve %d: change %v, line %q", i, rec.Change,
				rec.SharedError)
		}
	}
}

func TestFatalStartLine(t *testing.T) {
	for line, want := range map[string]bool{
		configLine: true,
		"panic: runtime error: invalid memory address":        true,
		"exec /usr/bin/sh: exec format error":                 true,
		"Exception in thread \"main\" java.lang.Error: boom":  true,
		"ModuleNotFoundError: No module named 'app'":          true,
		"[FATAL] license must be specified":                   true,
		"level=fatal msg=\"missing environment variable DB\"": true,
		"Error: Cannot find module '/app/server.js'":          true,
		"setting API_KEY must be set":                         true,
		"AccessDenied: User: arn:aws:sts::1:assumed-role/app is " +
			"not authorized to perform: s3:GetObject": true,
		"An error occurred (AccessDeniedException) when calling " +
			"the GetSecretValue operation": true,
		"WebIdentityErr: failed to retrieve credentials":     true,
		"NoCredentialProviders: no valid providers in chain": true,
		"googleapi: Error 403: PERMISSION_DENIED: caller lacks " +
			"permission": true,
		"AuthorizationFailed: The client does not have " +
			"authorization to perform action": true,
		"open /app/cache/index: read-only file system": true,
		"operation error STS: AssumeRoleWithWebIdentity, https " +
			"response error StatusCode: 403, api error " +
			"AccessDenied: Not authorized to perform " +
			"sts:AssumeRoleWithWebIdentity": true,
		"access log written to /var/log/app":            false,
		"mounted /config read-only as expected":         false,
		"GET /health 200 in 3ms":                        false,
		"retrying in 5s":                                false,
		"entering critical section":                     false,
		"dial tcp db:5432: connect: connection refused": false,
		"Unhandled exception. Npgsql: Connection to db:5432 " +
			"refused": false,
	} {
		if got := FatalStartLine(line); got != want {
			t.Errorf("FatalStartLine(%q) = %v, want %v", line, got, want)
		}
	}
}

// A container that exited cleanly, or never restarted, did not stop at
// start, whatever its log says.
func TestStartupErrorNeedsAnErrorExit(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("shop", "api", 1)
	startFails(f, pods, configLine, 0)
	f.fail(containerOf(pods[0]), "CrashLoop", failingH, 2, "")
	if f.snapshot().startupLine(Cause{
		Root: inventory.CoreID(kube.KindDeployment, "shop", "api"),
	}) != "" {
		t.Fatalf("quoted a start error after a clean exit")
	}
}

// A probe aimed at a port the spec does not declare is not blamed when
// the port answered: an HTTP status means something listens there.
func TestProbePortMismatchNeedsARefusedConnection(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("shop", "api", 1)
	id := containerOf(pods[0])
	f.observe(id, map[string]inventory.Value{
		kube.AttrContainerPorts: inventory.Text("8080"),
		kube.AttrProbePorts:     inventory.Text("8081"),
	})
	f.fail(id, "Probe.Readiness", failingH, 2,
		"Readiness probe failed: HTTP probe failed with statuscode: 500")
	f.fail(pods[0], "NotReady", degradedH, 2,
		"Get \"http://10.0.0.1:8081/readyz\": context deadline exceeded")
	if c, ok := f.explain().CauseOf(pods[0]); ok &&
		c.Row == "probe-port-mismatch" {
		t.Fatalf("blamed the probe port although it answered 500")
	}
}

// Both probe kills, 137 and 143, are symptoms of a fatal line that came
// first, and a missing required setting is such a line.
func TestStartupErrorAfterAnyProbeKill(t *testing.T) {
	for _, code := range []float64{137, 143} {
		f := newFixture(t)
		pods := f.workload("shop", "api", 2)
		startFails(f, pods, "Unhandled exception. System.Invalid"+
			"OperationException: persistence requires connection "+
			"string 'Store'.", code)
		liveKill(f, pods, livenessGives, neverReady)
		if c, ok := f.explain().CauseOf(containerOf(pods[0])); ok &&
			probeRows[c.Row] {
			t.Fatalf("exit %v: blamed %s", code, c.Row)
		}
	}
}

// Only a run that ended soon after it started stopped at start; a run
// that served for hours did not, and missing times do not rule it out.
func TestStoppedAtStart(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	run := func(d time.Duration) inventory.Entity {
		return inventory.Entity{Attributes: map[string]inventory.Attribute{
			kube.AttrLastStarted:  {Value: inventory.Time(t0)},
			kube.AttrLastFinished: {Value: inventory.Time(t0.Add(d))},
		}}
	}
	for name, tc := range map[string]struct {
		e    inventory.Entity
		want bool
	}{
		"ten seconds":    {run(10 * time.Second), true},
		"two minutes":    {run(2 * time.Minute), true},
		"three days":     {run(72 * time.Hour), false},
		"no times known": {inventory.Entity{}, true},
	} {
		if got := stoppedAtStart(tc.e); got != tc.want {
			t.Errorf("%s: stoppedAtStart = %v, want %v", name, got, tc.want)
		}
	}
}
