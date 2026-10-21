# Add a labelled scenario

A scenario is a recorded cluster story plus a label that says what kwatch must
conclude. The scorecard replays all of them, so a new scenario is both a
regression test and part of the quality gate. Real example:
`certificate-expired` in `internal/scenarios/library_certificate_test.go`.

1. **Write the generator.** In the `library_*_test.go` file for the area, add
   a function returning `scenario{expect: expectation{...}, build: ...}`.
   `build` drives a fake cluster with `c.list`, `c.update`, `c.warn`,
   `c.after(d)` and the object helpers (`c.deployment`, `w.pod`,
   `crashLoop`). Use fixed time only through `c.after`; nothing reads the wall
   clock.
2. **Label it.** Fill the expectation: `Name`, `Description`, `Root` (the
   entity that must be blamed, as `kind/namespace/name`, or `unknown`),
   `Tier`, `MaxMessages`, and `MustNotBlame` for the entities that would be
   the tempting wrong answer. A scenario that must stay silent sets
   `Quiet: true` instead of a root. Add a negative twin for every positive
   one (`certificate-expiring-app-crash`).

   `MustNotBlame` is checked against every stated cause, the rival of a
   hedged cause and the sentence of a message that names an entity as the
   cause. Beyond it, the first message may only state the label's roots
   (`Root`, `OtherRoots`, `Unrelated`) or no cause. Three optional fields
   say when a scenario is allowed to differ:

   - `RevisesRoot: true`: the story is a legitimate revision, so the first
     message may name a root that a later one replaces (a failing
     container named before its Deployment, a cause that arrives late).
     Use it for a story, never to silence a wrong first answer.
   - `Ambiguous: true`: the evidence cannot separate the root from a
     rival. A cause that names that rival as a hedge is not a wrong root
     stated at likely confidence; one that states a wrong root alone is.
   - `Hidden: [...]`: kinds kwatch never syncs, as when its RBAC lacks the
     permission. Detectors skip them and the message names the blind spot.
     (`newDependenciesLosingSight` models a kind that was visible and then
     lost, which is a different story.)

   With several roots, list them in any order: independent incidents are
   matched one by one.
3. **Register it.** Add it to the area's `...Scenarios()` slice, or add a new
   slice to `library()` in `scenario_test.go`.
4. **Generate the files.** This writes `testdata/<name>.jsonl` (the
   observation log) and `testdata/<name>.expect.json` (the label). Review both
   in the diff; the label is the contract.

   ```sh
   go test ./internal/scenarios -run 'TestScenarioFixtures/<name>$' -update
   go test ./internal/scenarios -run 'TestScenarioMessagesGolden/<name>$' -update
   go test ./internal/scenarios -run TestScenarioReplay -v -scenario <name>
   ```

   The second command prints each message so you can read what people would
   see.
5. **Run the scorecard.** The gates in
   [production goals](./production-goals.md) must still pass: correct root,
   no wrong root stated at likely confidence or higher, a right first
   message, messages per incident, notifications per hour and the storm.

```sh
go test ./internal/scenarios -run 'TestScenario'
make alert-quality-gate
```

`make alert-quality-gate` runs two checks. `TestScorecardGates` fails when a
gate misses. `TestScorecardGatesSeeRootCauseRegressions` is the mutation
check: it switches a part of the root-cause engine off (a scorer, the
vetoes, the counterfactual checks) and fails when no gate notices. A new
scorer or veto that no scenario depends on shows up there, so add a scenario
that needs it, or an entry in `ablations`
(`internal/scenarios/ablation_gates_test.go`).

A wrong verdict found in the field becomes a permanent scenario the same way.
Do not lower a gate or relax a label to make a scenario pass; fix the table,
weights or detector instead. Results are summarised in
`internal/scenarios/SCORECARD.md`: its "Current report" block is the output
of `make alert-quality` pasted between the markers, and
`make alert-quality-gate` fails when that block has drifted, so refresh it
in the same change.

## Guards

`make verify` (and so CI) blocks these mistakes with tooling:

- **Scoped `-update` only.** `-update` refuses to run unless `-run` names
  specific scenarios (`-run 'TestScenarioMessagesGolden/bad-rollout$'`). An
  empty or match-everything filter fails with instructions. A deliberate full
  regeneration, for an integration pass, needs `KWATCH_UPDATE_ALL=1`.
- **Fixtures and goldens match the registry.** `TestScenarioFilesMatchRegistry`
  fails on a log, label or message golden with no registered scenario, and on
  a registered scenario missing a file. Fixture and golden tests compare bytes
  with the generator output, so a hand edit, a missing trailing newline or a
  stale file fails.
- **Hygiene** (`make hygiene-check`, `internal/architecture/hygiene_test.go`).
  Fails on `zz_*_test.go`, `*_tmp_test.go`, `*_debug_test.go`, `*.orig` and
  `*.rej` files, tracked or untracked and not ignored; on the forbidden vendor
  word in any text file; on Go files that are not gofmt-formatted; and on Go
  files of 400 lines or more (three older files are allowlisted at their
  current length and may not grow). The 80-column rule stays in `line-check`.
- **Pinned linter.** `GOLANGCI_VERSION` in the Makefile is the only place the
  golangci-lint version is set; `make lint` builds it with the active Go
  toolchain, locally and in CI.

## The ratchet

The aggregate gates average over hundreds of scenarios, so one scenario that
stops passing can hide inside the slack.
`internal/scenarios/testdata/ratchet.txt` lists every labelled scenario that
passes, with how its root was found
(`found-high`, `found-likely`, `found-possible`, `self`, `unknown`, `quiet`).
`TestScenarioRatchetHolds` and the scorecard gate "Scenarios that passed and
no longer do" fail when a listed scenario fails, is gone, or states its cause
at a lower confidence level than the list records (a cause that falls back to
"no cause" still reads as the right root otherwise). A scenario that is not
listed is not held to anything yet.

The list moves only on purpose. After a change that should alter it,
regenerate it and read the diff: a removed line is a regression you are
accepting, an added line pins a new scenario.

```sh
go test ./internal/scenarios -run TestScenarioRatchetHolds -update-ratchet
```

A scenario that opts out of the first-message gate with `RevisesRoot` is
still judged on its final root and still counts for the ratchet.

## Adversarial scenarios

`library_adversarial_*_test.go` holds scenarios written by someone who did
not build the engine, to find where it is confidently wrong: a recent change
that is not the cause, look-alike names, a healthy dependency, faults that
share a node, and states that must stay quiet. Their names start with
`adv-`, their labels say what a person who knows Kubernetes would conclude,
and they are ordinary members of the labelled library: fixtures and message
goldens live in `testdata` and the same gates judge them.
