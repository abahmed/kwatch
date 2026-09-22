//go:build e2e

package harness

type ReproductionClass string

const (
	FixedOnMain         ReproductionClass = "fixed_on_main"
	StillFailing        ReproductionClass = "still_failing"
	RegressionOnMain    ReproductionClass = "regression_on_main"
	NotReproduced       ReproductionClass = "not_reproduced"
	InvalidReproduction ReproductionClass = "invalid_reproduction"
	UnsupportedVersion  ReproductionClass = "unsupported_version"
)

type VersionRun struct {
	Version    string
	SourceSHA  string
	Image      string
	Reproduced bool
	Passed     bool
}

func ClassifyVersionRuns(reported, main VersionRun) ReproductionClass {
	if !reported.Reproduced {
		return NotReproduced
	}
	if !reported.Passed && !main.Reproduced {
		return InvalidReproduction
	}
	if !reported.Passed && main.Passed {
		return FixedOnMain
	}
	if !reported.Passed && !main.Passed {
		return StillFailing
	}
	if reported.Passed && !main.Passed {
		return RegressionOnMain
	}
	return NotReproduced
}
