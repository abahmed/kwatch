// Package coverage is the coverage backstop's memory. This comment is the
// one explanation of the whole check; the other files point here.
//
// Every failing workload should be spoken for by a live incident. When
// one is not, an incident was lost somewhere (resolved while its workload
// still failed, absorbed under another that closed), and nobody would be
// told. The check finds such a workload and gives its active findings
// back to the incident manager, so they follow the normal settle, tier
// and announce path.
//
// Three pieces cooperate:
//   - this package (Watch) remembers which workloads have been short of
//     replicas long enough to be believed (After), when the check last
//     ran (Every) and which workloads were just handed back (Retry);
//   - pipeline/coverage.go (Engine.checkCoverage) runs on every engine
//     step, asks the incident manager who is covered, and does the
//     handing back, because that needs the manager and the findings;
//   - incident/coverage.go (Manager.CoveredWorkloads) says which
//     workloads a live incident speaks for, at any tier.
//
// The package never imports the pipeline.
package coverage
