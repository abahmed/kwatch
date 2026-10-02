# Add a detector

A detector reads one object and returns its health. It never reads
Kubernetes, a clock or a client. Real example: `detectors.HPA` in
`internal/detection/detectors/hpa.go`, which reports an autoscaler that
cannot read its metrics.

1. **Name the reason.** Add a constant to
   `internal/detection/reasons/reasons.go`, as in
   `HPAMaxedOut = "HPAMaxedOut"`. Reasons are stable: people grep audit logs
   and route alerts by them.
2. **Give it a mode.** Add the reason to the area map in
   `internal/detection/health.go` that fits it (`containerModes`,
   `workloadModes`, `nodeModes`, ...). `modeByReason` merges those maps;
   HPA reasons live in `workloadModes`, as in `reasons.HPAMaxedOut:
   ModeScalingMaxedOut`. Every mode is a `detection.Mode` constant in
   `internal/detection/mode_names.go`; a new mode gets its constant there
   first. The mode is the short failure identity that the propagation table
   matches and the tracker compares. Modes nest by dots:
   `ImagePull.Registry` is a kind of `ImagePull`. A detector gets Health from
   severity (`Info` and `Warning` become degraded, `Critical` becomes
   failing); set `Health` on the finding only when that is wrong, for example
   `detection.Unknown`.
3. **Write the detector.** Implement `Name`, `Kinds` and `Detect` in
   `internal/detection/detectors/`. Read attributes with the helpers
   (`number`, `text`, `condition`), return `detection.Finding{Reason,
   Severity, Since, Summary, Evidence}`, and let the registry fill in the
   entity, health and mode. For a condition that depends on elapsed time call
   `ctx.RecheckAfter(d)`. When the kind may be unwatched, check
   `ctx.Synced(kind)` and return nothing: a missing source is never a
   recovery. Evidence is text people read; it is redacted when first recorded.
4. **Register it.** Add the detector, for example `HPA{}`, to the list in
   `Default()` in `internal/detection/detectors/default.go`. The application
   and every test harness build their registry from `Default()`, so this one
   line enables it everywhere. If the detector needs a new attribute, add the
   attribute name and its extraction to the kind's schema in
   `internal/inventory/kube` (see the [source guide](./contributing-source.md)).
5. **Choose its delivery tier.** The incident manager decides how loudly a
   finding is delivered in `internal/incident/policy.go`. A finding worth
   knowing but never worth an interruption on its own (like
   `HPAMaxedOut`) belongs in `digestReasons`. A critical finding that should
   page, not just notify, needs a rule in `pageRules` in
   `internal/incident/page_rules.go`. Everything else notifies by severity.
6. **Test it.** Add a table test beside the detector, like
   `TestHPAConditionReasons` in
   `internal/detection/detectors/hpa_test.go`: build a model with
   `newTestModel()` (from `fixtures_test.go`), set attributes, call `Detect`,
   and assert reasons for the positive, negative (healthy and unrelated) and
   unknown cases. The shared guards fail if a step is missing:
   `TestEveryReasonHasMode` (step 2, in `internal/detection/health_test.go`)
   and `TestDefaultDetectorsAreDistinct` (step 4, in `default_test.go`).

```sh
go test ./internal/detection/... \
  -run 'TestHPA|TestEveryReasonHasMode|TestDefaultDetectorsAreDistinct'
make verify-focused PKGS="./internal/detection/... ./internal/incident"
```

If the detector introduces a failure that can cause others, add a
[propagation row](./contributing-propagation-rule.md); that guide also adds
the plain-English wording a message uses for the cause (`causeWords`).
Regenerate [Kubernetes coverage](./kubernetes-coverage.md) with
`go run ./cmd/coveragedocs` when modes change.
