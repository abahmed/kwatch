package investigate

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

var (
	crashPod       = inventory.CoreID(kube.KindPod, "shop", "api-1")
	crashContainer = inventory.CoreID(kube.KindContainer, "shop",
		"api-1/app")
	dbService = inventory.CoreID(kube.KindService, "shop", "db")
	dbSlice   = inventory.CoreID(kube.KindEndpointSlice, "shop", "db-x")
)

// crashModel is a crash looping pod whose Service db has no ready
// endpoints.
func crashModel(t *testing.T) *inventory.Model {
	t.Helper()
	return testModel(t,
		observed(crashPod, map[string]inventory.Value{
			kube.AttrPhase: text("Running")}),
		observed(crashContainer, map[string]inventory.Value{
			kube.AttrRestarts:    num(4),
			kube.AttrLastReason:  text("Error"),
			kube.AttrStateReason: text("CrashLoopBackOff")}),
		related(crashContainer, inventory.PartOf, crashPod),
		observed(dbService, map[string]inventory.Value{
			kube.AttrClusterIP: text("10.96.14.7")}),
		observed(dbSlice, map[string]inventory.Value{
			kube.AttrEndpointsReady: num(0)}),
		related(dbSlice, inventory.Backs, dbService),
	)
}

var refusedLines = []string{"connecting to db:5432",
	"dial tcp 10.96.14.7:5432: connect: connection refused"}

func tailOf(lines []string, err error) LogTail {
	return func(context.Context, inventory.EntityID, bool) (
		[]string, error) {
		return lines, err
	}
}

func crashIncident() incident.Incident {
	p := incidentOf(crashContainer, finding(crashContainer, "CrashLoop"))
	p.Cause = &rootcause.CauseRecord{Root: dbService}
	return p
}

func sectionsOf(r Result, check string) []incident.Section {
	var out []incident.Section
	for _, s := range r.Report {
		if s.Check == check {
			out = append(out, s)
		}
	}
	return out
}

func TestInvestigateReportsEveryApplicableCheck(t *testing.T) {
	s := Sources{Model: crashModel(t), Tail: tailOf(refusedLines, nil)}
	_, r := planAndRun(t, s, crashIncident())
	for _, check := range []string{incident.CheckCrash,
		incident.CheckPodState, incident.CheckLogs, incident.CheckEvents} {
		if len(sectionsOf(r, check)) == 0 {
			t.Errorf("no %s section in %+v", check, r.Report)
		}
	}
	logs := sectionsOf(r, incident.CheckLogs)
	if logs[0].Status != incident.SectionOK || logs[0].Run != "previous" ||
		!slices.Equal(logs[0].Matched, []int{1}) {
		t.Fatalf("log section = %+v", logs[0])
	}
}

func TestInvestigateCorrelationConfirmsUnreachableService(t *testing.T) {
	s := Sources{Model: crashModel(t), Tail: tailOf(refusedLines, nil)}
	_, r := planAndRun(t, s, crashIncident())
	if r.Correlation == nil ||
		!strings.Contains(r.Correlation.Confirmed, "0 ready endpoints") {
		t.Fatalf("correlation = %+v", r.Correlation)
	}
	var labels []string
	for _, f := range r.Correlation.Facts {
		labels = append(labels, f.Label)
	}
	if !slices.Contains(labels, incident.CorrelationMatches) {
		t.Fatalf("no match fact in %v", labels)
	}
}

func TestInvestigateCorrelationNeedsACause(t *testing.T) {
	p := crashIncident()
	p.Cause = nil
	// The member is a container, not the root: nothing claims a cause.
	p.Root = crashPod
	s := Sources{Model: crashModel(t), Tail: tailOf(refusedLines, nil)}
	_, r := planAndRun(t, s, p)
	if r.Correlation != nil && r.Correlation.Confirmed != "" {
		t.Fatalf("confirmed without a cause: %+v", r.Correlation)
	}
}

func TestInvestigateLogsDegradeWhenDeniedOrOff(t *testing.T) {
	forbidden := apierrors.NewForbidden(
		schema.GroupResource{Resource: "pods/log"}, "api-1",
		errors.New("no"))
	tests := []struct {
		name    string
		sources Sources
		reason  string
	}{
		{"rbac", Sources{Tail: tailOf(nil, forbidden)}, incident.ReasonRBAC},
		{"no reader", Sources{}, incident.ReasonRBAC},
		{"off", Sources{Tail: tailOf(refusedLines, nil), LogsOff: true},
			incident.ReasonOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.sources.Model = crashModel(t)
			_, r := planAndRun(t, tt.sources, crashIncident())
			logs := sectionsOf(r, incident.CheckLogs)
			if len(logs) == 0 || logs[0].Status != incident.SectionDenied ||
				logs[0].Reason != tt.reason || len(logs[0].Lines) > 0 {
				t.Fatalf("log sections = %+v", logs)
			}
			if len(sectionsOf(r, incident.CheckPodState)) == 0 {
				t.Fatal("a denied read must not drop the other sections")
			}
		})
	}
}

func TestRunChecksMarksUnfinishedCheckTimedOut(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	stuck := check{name: "stuck", title: "Stuck", budget: MaxBudget,
		read: func(context.Context, Sources, incident.Incident) []incident.Section {
			<-release
			return nil
		}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := runChecks(ctx, Sources{}, incident.Incident{},
		[]check{stuck}, -1)
	if len(r.Report) != 1 || r.Report[0].Status != incident.SectionTimeout {
		t.Fatalf("report = %+v", r.Report)
	}
}

func TestRunChecksSurvivesAPanickingCheck(t *testing.T) {
	boom := check{name: "boom", title: "Boom", budget: MaxBudget,
		read: func(context.Context, Sources, incident.Incident) []incident.Section {
			panic("boom")
		}}
	fine := check{name: "fine", title: "Fine", budget: MaxBudget,
		read: func(context.Context, Sources, incident.Incident) []incident.Section {
			return []incident.Section{{Facts: []incident.SectionFact{
				{Label: "a", Value: "b"}}}}
		}}
	r := runChecks(context.Background(), Sources{}, incident.Incident{},
		[]check{boom, fine}, -1)
	if len(r.Report) != 1 || r.Report[0].Check != "fine" {
		t.Fatalf("report = %+v", r.Report)
	}
}

func TestBoundedCapsReportAndRedactsIt(t *testing.T) {
	var lines []string
	for i := 0; i < maxSectionLines+50; i++ {
		lines = append(lines, "line password=hunter2")
	}
	r := Bounded(Result{Report: []incident.Section{{Check: "logs",
		Lines: lines, Matched: []int{0, maxSectionLines + 49}}}})
	got := r.Report[0]
	if len(got.Lines) != maxSectionLines {
		t.Fatalf("kept %d lines", len(got.Lines))
	}
	if strings.Contains(strings.Join(got.Lines, "\n"), "hunter2") {
		t.Fatal("a report line kept a secret")
	}
	if !slices.Equal(got.Matched, []int{maxSectionLines - 1}) {
		t.Fatalf("matched = %v", got.Matched)
	}
}

func TestInvestigateStampsWhenEachCheckRead(t *testing.T) {
	s := Sources{Model: crashModel(t), Tail: tailOf(refusedLines, nil),
		Now: func() time.Time { return investigatedAt }}
	_, r := planAndRun(t, s, crashIncident())
	for _, sec := range r.Report {
		if !sec.At.Equal(investigatedAt) {
			t.Fatalf("%s read at %v", sec.Check, sec.At)
		}
	}
}

func TestInvestigateClassifiesFailedReads(t *testing.T) {
	notFound := apierrors.NewNotFound(
		schema.GroupResource{Resource: "pods/log"}, "api-1")
	tests := []struct {
		err  error
		want string
	}{
		{notFound, incident.SectionAbsent},
		{errors.New("connection reset"), incident.SectionError},
	}
	for _, tt := range tests {
		s := Sources{Model: crashModel(t), Tail: tailOf(nil, tt.err)}
		_, r := planAndRun(t, s, crashIncident())
		if got := sectionsOf(r, incident.CheckLogs)[0].Status; got != tt.want {
			t.Errorf("%v: status %q, want %q", tt.err, got, tt.want)
		}
	}
}

func TestUsageFallsBackToTheModelWithoutMetricsServer(t *testing.T) {
	model := testModel(t,
		observed(crashPod, nil),
		observed(crashContainer, map[string]inventory.Value{
			kube.AttrMemoryLimit: text("256Mi")}),
		related(crashContainer, inventory.PartOf, crashPod))
	s := Sources{Model: model, PodUsage: func(context.Context,
		inventory.EntityID) ([]kube.ContainerUsage, error) {
		return nil, errors.New("the server could not find the resource")
	}}
	p := incidentOf(crashContainer, finding(crashContainer, "OOMKilled"))
	_, r := planAndRun(t, s, p)
	usage := sectionsOf(r, incident.CheckUsage)
	if len(usage) == 0 || usage[0].Status != incident.SectionOK {
		t.Fatalf("usage = %+v", usage)
	}
}
