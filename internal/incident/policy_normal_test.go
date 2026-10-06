package incident

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func normalMember(
	reason string, sev detection.Severity, normal detection.Normality,
) detection.Finding {
	deploy := entity(kube.KindDeployment, "worker")
	f := sig(deploy, reason, sev)
	f.Normal = normal
	return f
}

func TestTierUsualRestartsWaitForTheDigest(t *testing.T) {
	p := incidentOf(entity(kube.KindDeployment, "worker"), nil,
		normalMember(reasons.HighRestartCount, detection.Warning,
			detection.NormalUsual))
	if got := tier(p); got != Digest {
		t.Fatalf("tier = %v, want Digest", got)
	}
}

func TestTierUnjudgedRestartsStillNotify(t *testing.T) {
	p := incidentOf(entity(kube.KindDeployment, "worker"), nil,
		normalMember(reasons.HighRestartCount, detection.Warning,
			detection.NormalUnjudged))
	if got := tier(p); got != Notify {
		t.Fatalf("tier = %v, want Notify", got)
	}
}

// History excuses only the finding that carries it: a crash loop of
// the same workload still interrupts.
func TestTierUsualRestartsNeverExcuseACrashLoop(t *testing.T) {
	p := incidentOf(entity(kube.KindDeployment, "worker"), nil,
		normalMember(reasons.HighRestartCount, detection.Warning,
			detection.NormalUsual),
		sig(entity(kube.KindDeployment, "worker"),
			reasons.CrashLoopBackOff, detection.Critical))
	if got := tier(p); got != Notify {
		t.Fatalf("tier = %v, want Notify", got)
	}
}

// Once the workload has been down past the boot window the incident is
// persistent, and a usual rate no longer keeps it in the digest.
func TestTierUsualStopsExcusingAPersistentOutage(t *testing.T) {
	p := incidentOf(entity(kube.KindDeployment, "worker"), nil,
		normalMember(reasons.HighRestartCount, detection.Warning,
			detection.NormalUsual))
	p.persistent = true
	if got := tier(p); got != Notify {
		t.Fatalf("tier = %v, want Notify", got)
	}
}

func TestTierUnusualRestartsAreNotRoutine(t *testing.T) {
	p := incidentOf(entity(kube.KindDeployment, "worker"), nil,
		normalMember(reasons.HighRestartCount, detection.Warning,
			detection.NormalUnusual))
	p.Occurrences = dailyOccurrences(3, 3, 3)
	if got := tier(p); got != Notify {
		t.Fatalf("tier = %v, want Notify for restarts far above normal",
			got)
	}
}

func TestTierRoutineCrashLoopStaysRoutineWhateverItsRate(t *testing.T) {
	p := incidentOf(entity(kube.KindDeployment, "worker"), nil,
		normalMember(reasons.CrashLoopBackOff, detection.Critical,
			detection.NormalUnusual))
	p.Occurrences = dailyOccurrences(3, 3, 3)
	if got := tier(p); got != Digest {
		t.Fatalf("tier = %v, want Digest", got)
	}
}
