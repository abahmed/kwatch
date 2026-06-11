## What this PR does

Completes the incident-centric transition (REFACTOR_PLAN Phases 0–7):
- **Phase 0**: B1–B7 correctness bugs (+ tests)
- **Phases 1–2**: engine-owned state; pure-Detect/IO-Enrich pipeline; zero API calls on the healthy path (regression-locked by counting tests)
- **Phase 3**: pods/nodes/PVC/jobs/cronjobs/daemonsets/rollouts/HPA/TLS through one dedup+resolve engine; persisted incident-level startup baseline
- **Phase 4**: leader election; true multi-namespace informers; workers knob
- **Phase 5**: pending/rollout/job/cronjob/daemonset/HPA/TLS signals (all disabled by default)
- **Phase 6**: hints v2 (OOM-vs-limit, exit codes, probe detail); severity map
- **Phase 7**: retry+fallback, silences, routes, templates, renotify, inhibition, storm digests, external dead-man heartbeat, /incidents, /test-alert, kwatch lint/replay
- **GitOps**: KwatchConfig CRD with live reload (CR > ConfigMap)

## Bug fixes in this PR

- **Escalation double-bump** (fa8438e): `escalateSeverity` called twice → every crossing landed on "critical". Fixed: base on `inc.Severity`, call once.
- **Inhibition dead flag** (fa8438e): `inhibition.nodeSuppressesPods` was never consulted → suppression was force-on. Fixed: gate with config flag.
- **Enricher overwrites RestartCount** (fa8438e): `Enrich()` set `inc.RestartCount = ev.RestartCount` (always 0), breaking escalation prev/cur comparison on every path. Removed.

## Notes for reviewers

- `d33135f`'s commit message overclaims ("B-P1 through B-P8") — superseded by later commits; review by diff, not message.
- Known accepted limitation: upgrade startup-message can be swallowed if a standby marks the new version first (cosmetic).

## Verification

- `go build ./...` — green
- `go vet ./...` — green
- `go test -race ./internal/...` — green (all 110+ tests passing)
- kind smoke: see §5 in the attached plan for manual checklist
