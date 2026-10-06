# Reopen and supersede

Part of [The life of an incident](../incident-lifecycle.md).

A resolved incident is reopened as the same incident (same ID, same
Slack thread) when all of these hold (`reopenable` in `reopen.go`):

- people were told: a message of its own reached someone (`Incident.CanReopen`
  reads the delivery record `sentMark.told`, not the settle time). An
  announcement that was held and dropped (resolved inside a startup summary,
  outage hold or digest window) or dropped as out of scope was never heard;
- its tier was at least `Notify`, or it is a held page (`Delivery.pageHeld`),
  or it is a digest-tier incident that a digest listed;
- it was not superseded;
- it resolved within `RepageWindow` (2h);
- the new finding's mode matches. `modes.go` keeps the set of every mode the
  incident's members ever had, and the finding must match one of them. A
  finding or incident with no mode matches any.

On reopen the tier becomes `Notify` (a digest-tier incident stays at the
digest) and a page is marked `pageHeld` (`HoldAtNotify`), so the
incident does not page again. A digest incident's `failing again` update is
not sent on its own: the next digest lists it as recurring (and drops the
resolve line it may still hold for it). It keeps the page's reminders. One `failing
again` update is sent after `ReviseSettle`, so the members that return are in
the same message. If the reopened incident recovers before that update is
due, the update is sent when the failure returns and the incident is `Open`
again (never while `Recovering`). After the window, or for another mode, the failure is a new
incident that remembers the old one. A reopen also forgets the fix attempt
of the earlier occurrence (`Attempt`, the unsent attempt and its late flag).

How paging state works now:

- `Delivery.OpenAtPagers()` (`MarkPaged` / `ClosePage`) means the alert is open at the paging and issue-tracker
  providers. Any message of the incident's own that is not a resolve (an
  announcement or an update, `scopePaging` in `pipeline/deliver.go`) sets it.
  The one exception is the `failing again` update of a reopened page
  (`failingAgainOfPage`: reason `failing again` and `Delivery.pageHeld`). It
  goes to chat only (`SkipPaging`) and does not set it. The original page
  is the one page of that outage; its resolve closed the alert, and
  `Paged` stays false after it. A later reminder or material update of the
  reopened incident, if the outage lasts, reaches the pagers as a notify
  and opens a new alert then.
- A page held in the startup summary or an outage hold is paged on its own
  when it is announced. If it was held at notify and its update later
  reaches the page tier, the hold pages it then (`risesToPage`) and marks it
  paged, so the page is not lost; an outage hold pages the namespace once.
  A listing (roll-up, outage message) never pages an incident whose alert is
  already open (`pagedAlready`).
- A quiet supersede (a resolve without a message) purges what is still held
  for that incident: the announcer's hold, the startup summary, the digest
  and the outage hold (`TakeQuietResolves` returns the IDs). A held
  announcement is never sent for an incident that was resolved quietly.
- The route of the incident (`Incident.AnnouncedRoute`: namespaces,
  finding reasons, severity) is recorded when the incident is announced and
  widened by every update that is sent (union of namespaces and reasons,
  highest severity), and persisted. Delivery routes the resolve with it
  whole after the members are gone, so a pager route that only a later
  update matched still hears the resolve, also after a restart.
- A restored held page whose alert is open at the pagers (`Paged`) is
  announced again to chat only (`Decision.PagedAlready`) and stays paged.
- A new incident on a root and mode whose alert key a live, revised incident
  still holds gets a numbered key suffix (`freeAlertKey`), so the two never
  update or resolve each other's alert.
- A resolve closes it, and the resolve goes to the pagers only when `OpenAtPagers()`
  was set. A resolve for an alert nobody opened is not sent to them.
- A page to a pager that is down is not redirected to chat. Chat already
  gets its own copy of the message, and a chat fallback cannot open or close
  an alert by key, so the pager keeps retrying the opening and the resolve
  until it takes them (`mustReachPrimary` in `delivery/delivery.go`). If the
  pager stays down, nobody is paged; the health of the provider says so.
- In a full provider queue a resolve takes the place of the newest routine
  job, never of another resolve or of a page announcement. A resolve that is
  dead-lettered, expires or is dropped is logged at error with its alert key
  and counted in `kwatch_delivery_resolves_lost_total`: its alert may stay
  open. A resolve sent to an issue tracker that has no issue mapped for the
  key logs a warning with the key.
- Delivery marks an incident paged when the message is handed over; if every
  pager then gives up on its messages for good, delivery tells the pipeline
  (`PageObserver`) and the flag is cleared, so a later resolve is not sent
  to a pager that never held the alert. A job still queued or retrying keeps
  the flag.
- A record restored from an older version that was announced counts as paged,
  because sending a resolve is the safe mistake.
- `isPage` (page tier or `pageHeld`) and `reachedPaging` (`isPage` or
  `OpenAtPagers()`) in `flags.go` keep a page's resolve from being dropped or demoted.

Slack and the reopen thread. A resolve of an incident that may reopen
(`Incident.CanReopen`: announced, notify tier or held page, not superseded)
carries `Message.ReopenWithin` (`RepageWindow`). The Slack provider keeps
the conversation after that resolve until the window has passed, instead of
forgetting it. A `failing again` update inside the window is a reply in the
old thread, and the root message is edited back from the resolved marker to
the failing one; the next resolve edits it to resolved again. After the
window the conversation is dropped (a later failure is a new incident with
a new root), and the kept thread, with its end time, is saved with the other
threads, so a restart does not break it. The kept threads count toward the
same size bound as the open ones. A resolve that cannot reopen, or a
roll-up member's, is forgotten at once.

When a cause revision moves every member of an announced incident to another
announced incident, the old one is resolved as `superseded`. If its failures
are the same ones people were just told about and it never paged, that
resolve is not sent.

---

Previous: [Recovering and the resolve hold](./resolve.md) | Next: [The timings](./timings.md)
