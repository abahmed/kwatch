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
3. **Register it.** Add it to the area's `...Scenarios()` slice, or add a new
   slice to `library()` in `scenario_test.go`.
4. **Generate the files.** This writes `testdata/<name>.jsonl` (the
   observation log) and `testdata/<name>.expect.json` (the label). Review both
   in the diff; the label is the contract.

   ```sh
   go test ./internal/scenarios -run TestScenarioFixtures -update
   go test ./internal/scenarios -run TestScenarioReplay -v -scenario <name>
   ```

   The second command prints each message so you can read what people would
   see.
5. **Run the scorecard.** The gates in
   [production goals](./production-goals.md) must still pass: correct root,
   no wrong high-confidence root, messages per incident, notifications per
   hour and the storm.

```sh
go test ./internal/scenarios -run 'TestScenario'
make alert-quality-gate
```

A wrong verdict found in the field becomes a permanent scenario the same way.
Do not lower a gate or relax a label to make a scenario pass; fix the table,
weights or detector instead. Results are summarised in
`internal/scenarios/SCORECARD.md`.
