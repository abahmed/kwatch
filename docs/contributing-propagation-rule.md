# Add a propagation rule

A rule is a row in the propagation table plus a fixture test. Failure spread
is data, so no solver code changes. Real example: the `certificate-expired`
row in `internal/rootcause/explain/table_rows_traffic.go`.

1. **Add the row.** Pick the file by area (`table_rows_traffic.go`,
   `table_rows_lifecycle.go`, ...) and add a `Row` to its slice:

   ```go
   {Name: "certificate-expired",
    Cause: Side{Kind: kube.KindSecret,
        Modes: []detection.Mode{detection.ModeCertExpired}},
    Link:  LinkUses,
    Effect: Side{Kind: kube.KindPod, Signal: SignalTLS,
        Modes: append([]detection.Mode{detection.ModeFailed,
            detection.ModeError}, podFailures...)},
    Prior: 0.8},
   ```

   `Modes` are detector modes. They are defined, one map per area, in
   `internal/detection/health.go` (`containerModes`, `serviceModes`, ...,
   merged into `modeByReason`), each named by a constant in
   `mode_names.go`; a row can only match a mode a detector reason maps to
   there, or one of explain's pseudo modes such as `ModeChanged`. A mode
   also matches its finer modes: `ImagePull` matches `ImagePull.Registry`.
   The certificate modes are in `controlPlaneModes`. `TestTableModesAreKnown`
   fails for a mode no finding can carry. `Signal` demands error text
   on the effect, which stops an unrelated crash from being blamed on the
   cause. Generic rows have lower priors (0.5 to 0.7) so a specific row
   wins when both match. A new file's slice must be added to
   `specificRows` in `table_rows.go`.
2. **Add a fixture.** In the matching `table_rows_*_test.go`, add a `rowCase`
   to the group (`trafficRowCases`): a `build` function that uses the fixture
   helpers (`f.workload`, `f.fail`, `f.relate`, `f.change`) and returns the
   effect, and `want`, the root that must be blamed. A new group must be
   listed in `TestTableRows` in
   `internal/rootcause/explain/table_rows_specific_test.go`.
3. **Add a guard test.** Write the negative case that keeps the row honest,
   like `TestCertificateNeedsTLSErrors`: the same cause next to a failure
   with unrelated text must not be blamed.
4. **Word the cause.** Add the row name to `causeWords` in
   `internal/notification/compose/causes.go`. It is the plain-English text
   a message shows after the cause's name, for example
   `"certificate-expired": {words: "holds an expired certificate"}`. Set
   `own: true` when the cause is the subject's own configuration (see
   `memory-limit-too-low`). `TestCauseWordsCoverEveryRow` in
   `causes_test.go` fails for a row with no wording.
5. **Add a scenario.** Record an end-to-end case with the
   [scenario guide](./contributing-scenario.md), as
   `certificate-expired` does, including a negative one
   (`certificate-expiring-app-crash`).
6. **Run the gates.** `TestTableRows` fails for any row without a fixture,
   `TestTableModesAreKnown` for a misspelt mode, and
   `TestCauseWordsCoverEveryRow` for any row without wording.

```sh
go test ./internal/rootcause/explain \
  -run 'TestTableRows|TestTableModesAreKnown|TestCertificate'
go test ./internal/notification/compose -run TestCauseWordsCoverEveryRow
go test ./internal/scenarios -run TestScenarioFixtures
make alert-quality-gate
```

Weights live in `weights.go`; change one only with a reason and a new
scenario that needs it. Read the solver's reasoning for a case with
`go test ./internal/scenarios -run TestScenarioReplay -v -scenario <name>`.
