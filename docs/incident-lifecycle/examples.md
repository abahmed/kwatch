# Examples

Part of [The life of an incident](../incident-lifecycle.md).

## Example: a crash loop

1. 10:00:00 a pod of `api` starts crash-looping; a finding opens an
   incident (`Settling`).
2. 10:01:15 the settle is over: `Announce`, notify tier, one message.
3. 10:03 a second pod fails and the crash-loop stage is reached: the
   fingerprint changed, so `Update` (`material change`).
4. 10:08 someone edits the Deployment: `Update` (`fix attempt`).
5. 10:12 the pods are ready. The incident is `Recovering`; the hold is 3m.
6. 10:15 still healthy: `Resolve` (`healthy for 3m0s`), crediting the fix.

## Example: a flapping webhook

A validating webhook fails every ~13 minutes.

1. First failure: announced after the settle, at page tier if the page rule
   for rejecting webhooks matches.
2. It recovers and the hold (3m) passes: `Resolve`.
3. It fails again 10 minutes later, inside `RepageWindow`: the same incident
   reopens at notify (a held page), one `failing again` update.
4. Each recovery now counts as a cycle; the hold doubles, and as the webhook
   reopened within `ChronicWindow` it is multiplied by `ChronicFactor`, so
   the hold outlasts the gap between failures and "healthy again" stops
   alternating with "failing again".
5. After `FlapCycles` (3) cycles it is `Flapping`: one note, then silence
   unless it grows, until it is stable for `MaxHold` and resolves.

## How a crash storm with one shared error becomes one incident

Ten workloads crash with the same line, `license key rejected for tenant
4812`, each in its own pod.

1. Where the line comes from. The container's termination message is used
   first (`AttrLastMessage`). If it is empty, the crash-log round
   (`inventory/kube/crash_log_round.go`, every 30s, at most 20 reads) reads
   the previous run's log and stores the first error line that is not a stack
   frame as `AttrLastErrorLine` (`FirstErrorLine`).
2. `detectors/container.go` puts that text in the finding as `error`
   evidence, quoted as it is.
3. `explain` reads only that `error` evidence (`signatureOf`). It runs
   `format.Signature`, which lowercases and replaces times and IDs with
   placeholders. A word made only of hex letters and digits becomes `#`;
   any other word keeps its letters and only its digit runs become `#`
   (`worker7` reads `worker#`). IP literals become `<ip>` and keep a
   service port below 32768; a named host keeps its service port. A
   three-digit HTTP status after `http` or `status` is kept. The line is
   kept whole, so a leading `ERROR` stays, as `error`. Every replica and
   workload gives `error license key rejected for tenant #`.
4. `normalizeSignature` drops a signature that is on the generic list
   (`error`, `panic`, `exit status #` and so on), shorter than 16
   characters, under 3 words, or made only of generic network words. It cuts
   the rest to 80 characters.
5. The text, with the failing mode family in front (`signatureName`), names a
   virtual `failure-signature` entity. Each failing pod is linked to it
   (`LinkSharesError`).
6. The `shared-failure-signature` row proposes that entity as the cause once
   `SignatureMinWorkloads` (3) workloads that began failing within
   `SignatureWindow` (30m) share it (`applySignatureWindow`). It is the
   weakest row (prior 0.55), so a named endpoint or a better cause wins.
   The incident manager then attaches every finding to the incident rooted
   at the signature. The per-workload incidents that were settling are
   replaced by it, and one message says the shared error is the cause. The
   full settle (75s) helps: in a burst even a page waits for it.
7. The message (`compose/calls.go:signatureLead`, `compose/proof.go`) says
   `api in shop and nine other workloads keep crashing with the same
   error.` and then quotes one pod's own line: `It fails with "ERROR license
   key rejected for tenant 4812".` The quoted tenant number is that pod's,
   not the normalised `#`. With no other workload it says `api in shop
   keeps crashing with an error other workloads share`.

Worked example, from the first crash to the message:

```text
10:00:00  pod api-1 exits; log line "ERROR license key rejected for tenant 4812"
10:00:30  crash-log round stores it as last.error.line
10:00:31  finding "error" evidence; signature "CrashLoop error license key rejected for tenant #"
10:01:15  api-1 alone has settled and is announced as its own incident
10:03:00  api-2 and api-3 fail with tenant 5190 and 7731: same signature, 3 workloads
10:03:00  shared-failure-signature row proposes the signature entity as the cause
10:04:15  settle ends (75s): one announce, rooted at the signature; api-1's
          incident is re-rooted under it with the two new ones
```

---

Previous: [The timings](./timings.md) | Next: [What shapes what people hear](./shaping.md)
