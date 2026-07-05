# Coverage Progress

## Current Status
- **handler**: 96.9%
- **correlation**: 82.3%
- **health**: 77.7%
- **All packages**: all pass, no failures

## Achieved
- Handler coverage **78% → 96.9%** through systematic white-box testing of all major resource processors and edge cases
- **Generic lister error returns** now covered for all 14 Process functions via `error*Lister` mock wrappers (embedded real listers, override Get/List to return errors)
- **Lister.List error paths** in `SweepControlPlane` and `SweepTLSSecrets` now covered with dedicated error listers
- All new tests use testify/assert, fake clientsets, informer factories, and same-package access
- Tests cover key-based and object-based processing paths, nil/deleted/healthy/unavailable states, sustained windows, rollout grace periods, node inhibition, event lister paths, OOM repeating, init containers, ImagePullBackOff with/without auth secrets, CrashLoopBackOff with liveness probes, TLS certificate sweep, control-plane detection, all setter methods, and all lister error paths

## Remaining Gaps
- **emitHighRestartAlert** (80.0%): signalEvent call when owner is resolved — `correlation.ResolveOwnerName` returns empty for pods without matching lister entries
- **buildContainerHint** (85.4%): remaining branches need specific container state/reason combinations (e.g., OOMKilled with no limit, CrashLoopBackOff without liveness probe, init container error with exit code 0)
- **executeContainersFilters** (80.8%): event fetching branches (eventLister/k8s.GetPodEvents), enricher-stop, klog lines, signalEvent at end
- **executePodFilters** (82.3%): same event/enricher branches as above
- **DetectCronJobIssue** (88.9%): missing scheduled-time calculation branch
- **DetectControlPlanePodIssue** (96.9%): Running container continue branch
- **ProcessPod** (92.3%): remaining edge case
- **ProcessServiceObject** (93.3%): remaining edge case
- **ProcessIngressObject** (93.8%): remaining edge case

## Key Insights
- `cache.SplitMetaNamespaceKey` does NOT return an error for empty string or single-part keys
- `SetSeen` filters entries by BaselineTTL; default 0 TTL discards all entries
- `SetActiveNodeIncidents` appends to map (does NOT clear it first)
- `Process()` returns a snapshot; `SetAnalysis` modifies internal state directly
- MutatingWebhookConfiguration/ValidatingWebhookConfiguration are cluster-scoped (key = just name)
- Node key for ProcessNode is just the name (no SplitMetaNamespaceKey)
- Lister error paths can be tested via embedded mock wrappers that override Get/List on real informer-factory listers
