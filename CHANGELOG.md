# Changelog

All notable changes to this project are documented in this file.

## [Unreleased]

### Fixed

#### Phase 0 bugs

- **CR-1**: LifecycleHook data race — `MarkResolved`, `RemovePod`,
  `ResolveByResource`, and `checkLifecycle` renotify now clone the incident
  under lock before passing it to the hook callback.
- **CR-2**: Node inhibition cleared during hold-down — `ResolveByResource` no
  longer unconditionally deletes `activeNodeIncidents`; the hold-down pathway
  re-inserts the inhibition entry when a resource-level incident remains open.
- **PVC-3**: Swallowed node-level error auto-resolves PVCs — added
  `incomplete` flag; `getNodeUsage` errors set `incomplete=true` and the
  resolve-by-absence path skips incomplete cycles.
- **BUG-1**: CronJob false positives — split nil `LastScheduleTime` and
  staleness branches; new CronJob only alerts if `CreationTimestamp` > 24h ago;
  stale check only fires when `LastScheduleTime` is non-nil and > 24h old.
- **BUG-2**: PodStatusFilter "Added" casing — switched to
  `strings.EqualFold(ctx.EvType, "Added")`.
- **BUG-3**: OOMKilled hint for containers with no memory limit — added
  dedicated hint string when `resources.limits.memory` is nil or zero.
- **BUG-4**: Per-signal `IncludeEvents`/`IncludeLogs` overwritten by
  `eventWithConfig` — removed dead `IncludeEvents`/`IncludeLogs` fields from
  the `Signal` struct.
- **BUG-5**: `tls_sweep` clock — changed `time.Until(expiry)` to
  `expiry.Sub(now)`.
- **BUG-6**: Init container failures now use `InitContainerError` hint — added
  `IsInit bool` to `ContainerContext`, set when iterating
  `InitContainerStatuses`, and `buildContainerHint` prefers the
  `InitContainerError` hint when `IsInit && exitCode != 0`.
- **BUG-7**: Crashloop incidents dropped on transient clean exit — added
  `!ctx.Container.HasRestarts &&` guard to the skip condition for
  `Terminated{Completed|ExitCode 0|143}`.
- **F2**: Drop-oldest channel send can block — the drain-receive path is now
  non-blocking (`select` with `default`).
- **SCAN-1**: `ActiveCount` overcount — now iterates `e.state` counting only
  `State != StateResolved` instead of returning `len(e.state)`.
- **SCAN-3**: AlertManager `Init` races with concurrent `NotifyIncident` calls
  — added `a.mu` mutex; `Start` snapshots entries; `Notify`, `NotifyEvent`,
  `NotifyIncident` snapshot entries; `shutdown` is idempotent.
- **LOG-1**: Container detection log level — changed `klog.InfoS` to
  `klog.V(2).InfoS` in `execute_containers_filters.go`.
- **SQ-1**: Remove `startupQuiet` — deleted field from config and engine;
  removed wiring in `main.go` and CRD types.
- **PVC-1**: Kubelet proxy calls use request context — threaded
  `context.Context` through `PvcMonitor.checkUsage` → `getNodeUsage` →
  `GetNodeSummary`/`GetNodes`/`GetPVNameFromPVC`/`GetPodContainerLogs`/`GetPodEvents`;
  each wraps the call with `context.WithTimeout(ctx, 10s)`.
- **PVC-2**: N+1 PVC API fan-out — `checkUsage` builds a `pvByPVC` map
  (ns/name → spec.VolumeName) once per cycle via one `List` call.
- **PVC-4**: Nil PodRef panic — added `if pod.PodRef == nil { continue }`.
- **PVC-5**: Division by zero — added `if vol.CapacityBytes <= 0 { continue }`.
- **HTTP-1**: Discord/Telegram no timeout — replaced `&http.Client{}` with
  `k8s.GetDefaultClient()` (shared client with proper transport and timeout).
- **HTTP-2**: Unbounded log fetch when `maxRecentLogLines == 0` — default tail
  of 500 lines and 1 MB `LimitBytes` cap.
- **HB-1**: Heartbeat ping not tied to request context — `ping()` now accepts
  `context.Context` and creates the HTTP request with `NewRequestWithContext`.
- **MAIN-1/MAIN-2**: Shutdown/exit — replaced `os.Exit(0)` with a `stop`
  channel; lost-leader exits with code 1; graceful shutdown uses a fresh
  timeout context.
- **HEALTH-1**: `/readyz` static OK — added `ready atomic.Bool`;
  `SetReady(true)` at end of `runLeaderTasks`; returns 503 when not ready.
- **Severity default table**: Added `defaultSeverityByReason` map with
  `Evicted → medium`, `ImagePullBackOff → medium`. Looked up in
  `resolveSeverity` between user `SeverityByReason` and owner-kind check.
- **Escalation + default severities**: Added `severityForTier(tierIdx, current)`
  that computes tier-based severity, preferring the higher of the tier and the
  current severity. Prevents double-escalation when a default reason severity
  is already set.

### Added

- **NEW-4**: `internal/integration/controller_fault_test.go` — integration test
  (build tag `integration`) with fake clientset that verifies the controller
  tolerates processing pod events with no providers configured.

### Removed

- `correlation.startupQuiet` config field and CRD field.
- `IncludeEvents`/`IncludeLogs` fields from `event.Signal` struct (dead code).
- `StartupQuiet` field from `config/config.go` and `api/v1alpha1/types.go`.

### Changed

- `kubernetes.Interface` methods (`GetNodes`, `GetPVNameFromPVC`,
  `GetNodeSummary`, `GetPodContainerLogs`, `GetPodEvents`) now accept
  `context.Context` as the first parameter.
- `discord.Verify()`, `telegram.Verify()`, `telegram.sendByTelegramApi()` now
  use `k8s.GetDefaultClient()` instead of `&http.Client{}`.
- `HeartbeatMonitor.ping()` now requires a `context.Context` parameter.
- `PvcMonitor.checkUsage()` now requires a `context.Context` parameter.
