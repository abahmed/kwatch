# Add a detector

A detector reads one object and returns its health. It never reads
Kubernetes, a clock or a client. Real example: `detectors.HPA` in
`internal/detection/detectors/hpa.go`, which reports an autoscaler that
cannot read its metrics.

1. **Name the reason.** Add a constant to
   `internal/detection/reasons/reasons.go`, as in
   `HPAMaxedOut = "HPAMaxedOut"`. Reasons are stable: people grep audit logs
   and route alerts by them.
2. **Give it a table entry.** Everything kwatch knows about a reason is one
   `ReasonInfo` in the reason tables, `internal/detection/reason_table_*.go`
   (`reason_table_pod.go`, `_node.go`, `_workload.go`, `_service.go`,
   `_events.go`, `_upstream.go`, `_admission.go`). HPA reasons are in
   `reason_table_workload.go`, as in
   `reason(reasons.HPAMaxedOut, ModeScalingMaxedOut).causeOnly()`. The entry
   names the mode and, with the chained modifiers in `reasoninfo.go`, the
   rest: `.digest()` (never interrupts alone), `.causeOnly()` (a cause,
   no news alone), `.waiting(sev)` and `.event(sev)` (a container waiting
   state or Warning event that is a failure by itself), `.progress()`,
   `.shown(kinds...)` (a Warning event reason that the state of those kinds
   already shows; a repeat of the same reason on another kind is reported as
   an unusual event) and `.label(...)`. `modeByReason` and every lookup such
   as `DigestOnly` or `EventSeverity` are derived from the table, so
   nothing else lists the reason. Every mode is a `detection.Mode` constant
   in `internal/detection/mode_names.go`; a new mode gets its constant
   there first. The mode is the short failure identity that the
   propagation table matches and the tracker compares. Modes nest by dots:
   `ImagePull.Registry` is a kind of `ImagePull`. A detector gets Health
   from severity (`Info` and `Warning` become degraded, `Critical` becomes
   failing); set `Health` on the finding only when that is wrong, for
   example `detection.Unknown`.
3. **Write the detector.** Implement `Name`, `Kinds` and `Detect` in
   `internal/detection/detectors/`. Read attributes with the helpers
   (`number`, `text`, `condition`), return `detection.Finding{Reason,
   Severity, Since, Summary, Evidence}`, and let the registry fill in the
   entity, health and mode. A finding that is advice about configuration
   rather than a failure (see `detectors.Risk`) sets `Advisory: true` and
   `Severity: detection.Info`: it is never announced, never opens an
   incident on its own, never changes a tier and never leads a message about
   a failure; a failure of the same workload may quote it as a consequence.
   For a condition that depends on elapsed time call
   `ctx.RecheckAfter(d)`; a finding that holds only inside a time window
   (events of the last quarter hour, a preemption of the last fifteen
   minutes) asks to be rechecked just after the window ends, because
   nothing else touches the entity then. When the kind may be unwatched,
   check `ctx.Synced(kind)` and return nothing: a missing source is never a
   recovery. Evidence is text people read; it is redacted when first
   recorded. A detector that panics is contained: it is named in
   `Evaluation.Panicked`, the engine keeps the findings that detector
   raised before (`Finding.Detector` says which they are), applies the
   other detectors' fresh results as usual and asks for another
   evaluation, so a bug never clears a finding.
4. **Write facts, not forecasts.** A summary says what is true now: what
   is failing, since when, and what Kubernetes said, quoted byte for byte.
   It never says what may, will or could happen ("pods may be evicted",
   "will fail on its next restart"), and it never guesses a cause ("a
   finalizer is likely blocking it"). The test
   `TestDetectorTextStatesFactsNotPredictions` scans every string the
   detectors and reason tables hold and fails on such a word. Status
   conditions are read by the one table in
   `internal/inventory/kube/condition_status.go` (`kube.ConditionRules`:
   the type, the status that means trouble, and whether the object is
   failing or degraded), which the generic and custom resource detectors
   and the ranking of nested conditions share; add a condition type
   there, not in a second list. The message of a condition in its failing
   state is quoted as evidence.
5. **Register it.** Add an `Entry` for the detector, for example
   `{Feature: feature.Autoscaling, Detector: HPA{}}`, to `Entries()` in
   `internal/detection/detectors/default.go`. The `Feature` is the capability
   that entitles it (`TestEveryEntryHasAFeature`). `Default()` and
   `Entitled()` are derived from `Entries()`; `newDetectorRegistry` in
   `internal/app/pipeline.go` and every test harness build their registry
   from it, and `app/detectors_parity_test.go` fails if production diverges.
   If the detector needs a new attribute, add the attribute name and its
   extraction to the kind's schema in `internal/inventory/kube` (see the
   [source guide](./contributing-source.md)).
6. **Choose its delivery tier.** The incident manager decides how loudly a
   finding is delivered in `internal/incident/policy.go`. A finding worth
   knowing but never worth an interruption on its own (like
   `ServiceUnused`) is marked `.digest()` in its table entry (reasons with a
   shared prefix, such as `UnusualEvent.*`, `Risk.*` and `NodeCondition.*`,
   are matched by prefix in `digestReason`). A finding that fires before
   anything failed but is a real cause of the failures that follow (like
   `HPAMaxedOut` or `VolumeUsageHigh`) is marked `.causeOnly()`: an incident
   made only of such findings is silent, and is announced once a real
   failure joins it. A critical finding that should page, not just notify,
   needs a rule in `pageRules` in `internal/incident/page_rules.go`.
   Everything else notifies by severity.
7. **Test it.** Add a table test beside the detector, like
   `TestHPAConditionReasons` in
   `internal/detection/detectors/hpa_test.go`: build a model with
   `newTestModel()` (from `fixtures_test.go`), set attributes, call `Detect`,
   and assert reasons for the positive, negative (healthy and unrelated) and
   unknown cases. The shared guards fail if a step is missing:
   `TestEveryReasonHasMode` (step 2, in `internal/detection/health_test.go`),
   `TestDefaultDetectorsAreDistinct` (step 5, in `default_test.go`) and
   `TestDetectorTextStatesFactsNotPredictions` (step 4,
   in `summary_facts_test.go`).

## Words kwatch classifies

kwatch quotes logs and events and does not interpret them. The few places
where it reads text to tell failures apart are fixed wording, and this is
the whole list; a new one needs a reason here:

- **Pod sandbox failures** (`sandboxClasses` in `event_kubelet.go`): IP
  address exhaustion and "network plugin not ready", matched in the runtime's
  message. The matched text is quoted as evidence; no CNI is detected.
- **Multi-attach** (`multiAttachWords` in `event_storage.go`): a
  `FailedAttachVolume` message that says the volume is still attached to
  another node is its own, milder failure.
- **Container configuration errors** (`configErrorWords` and
  `missingRefMessage` in `container.go`): the kubelet's "not found" and
  "couldn't find key" wording tells a missing reference from any other
  rejection; the kubelet's own text is quoted as evidence.
- **Exec format error** (`container_arch.go`): the exact string the kernel
  prints, next to the node's `kubernetes.io/arch` label. The summary states
  the node's architecture and quotes the line; it does not say the image
  has no build for it.
- **Quota refusals** (`quota_attach.go`): the API server's refusal text,
  matched to attach a refusal to the quota that caused it.

Every other classification is by reason, status, condition or attribute,
never by a phrase in a message.

```sh
go test ./internal/detection/... -run \
  'TestHPA|TestEveryReasonHasMode|TestDefaultDetectors|TestDetectorText'
make verify-focused PKGS="./internal/detection/... ./internal/incident"
```

If the detector introduces a failure that can cause others, add a
[propagation row](./contributing-propagation-rule.md); that guide also adds
the plain-English wording a message uses for the cause (`causeWords`).
Regenerate [Kubernetes coverage](./kubernetes-coverage.md) with
`go run ./cmd/coveragedocs` when modes change.
