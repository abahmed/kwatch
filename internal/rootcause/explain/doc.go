// Package explain is the root-cause engine of ADR 0011: a propagation
// table, a candidate walk, scorers and a set-cover solver. The incident
// manager solves through a Solver, which caches areas and solves again
// only those a change can affect.
//
// In: a Snapshot of the inventory and its active findings. Out: an
// Explanation that groups the failing entities into connected areas and
// names the fewest causes that cover each area, with a confidence, the
// alternatives and a reasoning trace.
//
// The steps are small and each lives in its own file:
//
//   - table.go lists how failure spreads, as plain data; the rows
//     live in table_rows*.go by area (control plane, access, traffic,
//     node lifecycle, operators, containers), next to the read-time
//     hops (virtual*.go) that reach their causes.
//   - self.go and self_config.go say why a workload is its own cause:
//     its own edit, its limits, its probes.
//   - walk.go walks upstream from every failing entity to find
//     candidates.
//   - scorer_*.go add or subtract evidence, one Contribution per
//     scorer with a rootcause.ProofCode and its numbers; weights.go
//     holds every weight with the reason for its value.
//   - solver.go picks the causes with a greedy weighted set cover.
//   - incremental.go caches areas between solves.
//   - adapter.go and history.go read the inventory: links, changes,
//     change-set outcomes and baselines; Record turns a Cause into the
//     rootcause.CauseRecord an incident keeps.
//
// Rows match finding modes by the detection.Mode constants; the modes
// explain reads itself for candidates without a finding (a change, an
// absence, a failed call) are its pseudo modes. A Cause's Summary and a
// Contribution's Text are for traces: writers read Row, Mode and Code.
//
// Explain is a pure function: it does no I/O and reads no clock.
package explain
