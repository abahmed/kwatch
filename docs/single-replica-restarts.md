# What happens to alerts when kwatch restarts

kwatch runs one replica (`replicas: 1`). When that pod restarts, for an
upgrade, a config change, a drain or a crash, kwatch is not watching until
the new pod is ready. This page says what you can rely on and what you
cannot. The mechanics of the gap are in
[Production operations](./production-operations.md).

## What you can rely on

- **Alerts that were queued are kept.** Every alert is written to the state
  file on the volume (the outbox) when a provider queue accepts it, and
  removed when the provider, or its fallback, accepted it. A planned stop
  flushes the outbox first; the next pod sends what is left, oldest first,
  before any new alert.
- **Open issues are kept.** Issues, their timelines and the thread ids of
  your chat messages are saved, so an issue that was announced before the
  restart is not announced a second time, and its later updates go to the
  same thread.
- **Nothing is sent twice by two pods.** Only the pod that holds the lock
  claims the state file; an old pod that lost it stops sending.
- **A restart is never silent about a failure.** After the restart kwatch
  lists the cluster again. An issue that is still happening is found again
  and keeps its place; a short grace after the restart keeps it from being
  closed just because its evidence has not been read yet.

## What is not guaranteed

- **Nothing is watched during the gap.** A failure that starts and ends
  while kwatch is down is not reported. One that starts in the gap is
  reported once kwatch is ready and has listed the cluster.
- **At least once, not exactly once.** If the pod dies after a provider
  accepted a message and before the outbox record was removed, that
  message is sent again after the restart.
- **A hard crash can lose the last moments.** The outbox is written by a
  background writer in small batches. A crash (not a planned stop) can lose
  alerts queued in the last moments before it.
- **The outbox has limits.** It holds at most 2048 messages and drops ones
  older than 24 hours; a drop is counted in the metrics, not silent.
- **Messages that were already dead-lettered stay gone.** A message that
  every provider refused for good is not retried after a restart.

If you need a signal while kwatch itself is down, use the heartbeat
(`config.heartbeatMonitor`), which is checked from outside the cluster.
