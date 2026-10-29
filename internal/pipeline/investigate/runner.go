package investigate

import (
	"context"
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// checkOutput is what one check returned.
type checkOutput struct {
	index    int
	summary  Result
	sections []incident.Section
}

// runChecks runs planned in parallel, each under its own budget, and
// waits until all returned or ctx ends. A check still running then
// gets a timeout section; one that panicked gets nothing. The summary
// is the lead check's, and the report lists sections in plan order.
func runChecks(
	ctx context.Context, s Sources, p incident.Incident, planned []check,
	lead int,
) Result {
	done := make(chan checkOutput, len(planned))
	for i, c := range planned {
		go func() {
			out := checkOutput{index: i}
			defer func() { done <- out }()
			defer recoverCheck(c.name)
			checkCtx, cancel := context.WithTimeout(ctx, c.budget)
			defer cancel()
			out.summary, out.sections = c.run(checkCtx, s, p)
			stamp(out.sections, s.Now)
		}()
	}
	outputs := make([]*checkOutput, len(planned))
	for range planned {
		select {
		case out := <-done:
			outputs[out.index] = &out
		case <-ctx.Done():
			return assemble(planned, outputs, lead)
		}
	}
	return assemble(planned, outputs, lead)
}

// recoverCheck logs a panicking check. Reading the model or a log must
// never take the process down.
func recoverCheck(name string) {
	if r := recover(); r != nil {
		klog.ErrorS(nil, "investigate: check panicked",
			"component", "pipeline", "check", name,
			"panic", fmt.Sprint(r))
	}
}

// assemble joins the outputs into one result; a missing output is a
// check that ran out of time.
func assemble(planned []check, outputs []*checkOutput, lead int) Result {
	var out Result
	for i, c := range planned {
		got := outputs[i]
		if got == nil {
			out.Report = append(out.Report, incident.Section{
				Check: c.name, Title: c.title,
				Status: incident.SectionTimeout})
			continue
		}
		if i == lead {
			out.Output, out.Evidence = got.summary.Output,
				got.summary.Evidence
		}
		out.Report = append(out.Report, got.sections...)
	}
	return out
}

// run reads the check. A check with a summary reports it as its one
// section.
func (c check) run(
	ctx context.Context, s Sources, p incident.Incident,
) (Result, []incident.Section) {
	if c.summary != nil {
		r := c.summary(ctx, s, p)
		own := sectionOf(c, r, ctx.Err())
		if c.read != nil {
			for _, extra := range c.read(ctx, s, p) {
				own.Facts = append(own.Facts, extra.Facts...)
			}
			own.Status = statusOf(own)
		}
		return r, []incident.Section{own}
	}
	sections := c.read(ctx, s, p)
	for i := range sections {
		sections[i].Check, sections[i].Title = c.name, c.title
		if sections[i].Status == "" {
			sections[i].Status = statusOf(sections[i])
		}
	}
	if len(sections) == 0 {
		sections = []incident.Section{{Check: c.name, Title: c.title,
			Status: incident.SectionAbsent}}
	}
	return Result{}, sections
}

// sectionOf turns a summary into its section.
func sectionOf(c check, r Result, err error) incident.Section {
	out := incident.Section{Check: c.name, Title: c.title,
		Lines: r.Output}
	for _, e := range r.Evidence {
		label := e.Kind
		if e.Subject != "" {
			label += " " + e.Subject
		}
		out.Facts = append(out.Facts,
			incident.SectionFact{Label: label, Value: e.Text})
	}
	out.Status = statusOf(out)
	if out.Status == incident.SectionAbsent && err != nil {
		out.Status = incident.SectionTimeout
	}
	return out
}

// statusOf is ok for a section that read something, else absent.
func statusOf(s incident.Section) string {
	if len(s.Facts) > 0 || len(s.Lines) > 0 {
		return incident.SectionOK
	}
	return incident.SectionAbsent
}

// readStatus classifies a failed API read.
func readStatus(ctx context.Context, err error) (status, reason string) {
	switch {
	case err == nil:
		return incident.SectionOK, ""
	case kube.IsDenied(err):
		return incident.SectionDenied, incident.ReasonRBAC
	case ctx.Err() != nil:
		return incident.SectionTimeout, ""
	default:
		return incident.SectionAbsent, ""
	}
}

// gated returns the sources with every API read waiting for one of the
// investigator's slots, so all investigations together keep at most
// maxAPIReads reads in flight.
func (r *RootInvestigator) gated() Sources {
	s := r.sources
	if tail := s.Tail; tail != nil {
		s.Tail = func(ctx context.Context, c inventory.EntityID,
			previous bool) ([]string, error) {
			var lines []string
			err := r.gate(ctx, func() (err error) {
				lines, err = tail(ctx, c, previous)
				return err
			})
			return lines, err
		}
	}
	if readyz := s.Readyz; readyz != nil {
		s.Readyz = func(ctx context.Context) ([]string, error) {
			var lines []string
			err := r.gate(ctx, func() (err error) {
				lines, err = readyz(ctx)
				return err
			})
			return lines, err
		}
	}
	if usage := s.PodUsage; usage != nil {
		s.PodUsage = func(ctx context.Context, pod inventory.EntityID) (
			[]kube.ContainerUsage, error) {
			var out []kube.ContainerUsage
			err := r.gate(ctx, func() (err error) {
				out, err = usage(ctx, pod)
				return err
			})
			return out, err
		}
	}
	return s
}

// gate runs read once a slot is free, or returns ctx's error.
func (r *RootInvestigator) gate(ctx context.Context, read func() error) error {
	select {
	case r.slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-r.slots }()
	return read()
}

// stamp sets when the sections were read.
func stamp(sections []incident.Section, now func() time.Time) {
	if now == nil {
		return
	}
	at := now()
	for i := range sections {
		sections[i].At = at
	}
}
