# Dashboard UI notes

Implementation notes for the embedded dashboard UI in
`internal/dashboard/web` (plain HTML, CSS and JavaScript, no build step,
strict CSP, nothing loaded from elsewhere). The published user guide stays
on [kwatch.dev](https://kwatch.dev/docs).

## Layout and reading order

The UI is written for a junior engineer on call and a busy SRE alike:
the first thing on each page says what is wrong in one sentence, the key
details sit right under it, and the rest waits behind a click or a tab.

- **Sidebar**: **Overview** (`/`, the landing page), **Issues**
  (`/issues`, with the count of active issues), **Map** (`/map`),
  **Activity** (`/activity`) and **kwatch** (`/kwatch`). The old `/insights` page is
  part of the Overview now; its address redirects to `/`. The old
  `/releases` page is part of the issues now ("Releases" below); its
  address redirects to `/issues`. Pages of prose are 1000px wide at most
  (`--max-app`, the brand's); the Issues table is wider (`--max-table`,
  90rem).
  The **sidebar dot** beside "kwatch" (`js/pages.js`, `KW.selfDot`)
  shows when kwatch itself needs a look: alerts failed to send in the
  last hour, alert links are off, it is still starting, it can't read
  pod logs or some kinds, or its last check is late. Every page reads
  `/api/status` and `/api/kwatch` once a minute for it; the tooltip and
  the screen-reader text list the reasons. Under the links: browser
  notifications, the Local / UTC switch and sign out.
- **Old addresses** keep working: the server (and the page, for Back to
  an old history entry) sends `/status` to `/`, `/resolved?…` to
  `/issues?status=resolved&…`, and `/` with list filters (`q`,
  `severity`, `namespace`, `state`, `age`, `since`, `sort`, `dir`,
  `group`, `kind`, `acked`, `status`, `offset`, `limit`, `page`,
  `pageSize`) to `/issues?…` with the same query, so bookmarks and links
  pasted in chat open the same list. An issue stays at `/issues/<id>`.
- **Every page** has the same head: the page name as the heading (the
  sidebar's word), one muted line of what it answers, and on the
  live pages "Live · updated just now" (see "Live push").
- **Phones** (under 700 px wide): the sidebar's notifications, times
  and sign out fold behind one "Settings" button, and the Issues
  filters (quick views, search, pickers, severity chips) behind one
  "Filters (2)" button, so rows show on the first screen. No page
  scrolls sideways at 390 px.
- **Formats** (`js/core.js`), the same on every page and in
  notifications: one number format (`KW.num`: "4,812"; numbers in
  tables are right-aligned, `.num`), one object format (`KW.obj`:
  "deployment/payments-api", the namespace shown on its own, "whole
  cluster" without one), one clock format (`KW.hm`: "14:02",
  "yesterday 14:02" or "12 Oct, 14:02", the exact time in both zones
  with seconds in the tooltip), one duration format (`KW.lasted`:
  "12 min", "3 h 10 min", "2 days"; "for 3 h" while it goes on) and one
  way to cut a long issue title (`KW.clampTitle`: two lines, the whole
  title in the tooltip; the issue page shows it whole).
- **Chips**: severity is a filled chip (`KW.severityChip`), state an
  outlined one (`KW.stateChip`), "Acknowledged" an outlined muted one.
  "Settled before alerting" and "Stopped tracking" are small muted
  labels, never a severity color.
- **Empty states** read plainly, in one muted sentence inside the card
  or box that would hold the data ("No node has an active issue.",
  "Nothing changed in the last hour.", "A quiet week: nothing opened
  or resolved."); a chart with nothing to draw is not drawn.

Visible words replace internal terms; the JSON field names do not change:

| API value | Shown as | Tooltip |
|:--|:--|:--|
| `severity: page` | Urgent | Alert level: alerted the on-call person |
| `severity: notify` | Needs attention | Alert level: sent a message |
| `severity: digest` | Low | Alert level: in the daily summary |
| `severity: silent` | Quiet | Alert level: not sent |
| `state` | Confirming, Happening now, Keeps coming back, Looks better, Resolved (`KW.STATE`, see "Issues table") | |
| `untold` on a settled issue | Settled before alerting | It recovered while kwatch was still confirming it, so no alert went out |
| `stoppedTracking` | Stopped tracking | kwatch stopped tracking this issue while it was still failing |
| `rootCause`, `summary.why` | Why (on the issue page), Likely cause | kwatch's best guess |
| `ackedBy`, `acked` | Acknowledged | acknowledged with the annotation: the object that carries it (the detail's `ack.since` says since when kwatch saw it) |
| correlation | What the checks agree on | |

**Settled before alerting** (`KW.UNTOLD`, `KW.isUntold`): an issue
whose `untold` is set and that is no longer confirming or failing
(resolved or looking better) recovered before kwatch told anyone. It
shows as a small muted label after the title on Issues rows and in
Activity, beside the severity on the issue page and in the drawer, as
the stepper's line "Settled before alerting — nobody was told." (the
steps between Confirming and Resolved are marked skipped, not passed),
and in "Who was told": "Nobody was told: it settled before alerting."
A confirming issue carries `untold` too (nobody was told yet); it reads
as Confirming, not as settled. The Overview never counts
these as pages: their numbers come from the server, which leaves untold
openings out of `paged` and of the urgent counts.

## Overview

`/` (`js/status.js`, from `/api/overview` and `/api/status`) answers
"is anything broken right now, and what do I look at first", top to
bottom:

1. **Notices**, only when someone must act (`KW.selfNotices`, shared
   with the kwatch page): "kwatch is loading what it knows" with a bar
   of the kinds read (from `self.startup`, until `startup.done`) and
   "This page updates by itself.", shown only while there is no data
   yet (no issue to list), "kwatch hasn't
   checked your cluster in a while", "Alert links are off" with
   `dashboardLinks.text`, and "2 alerts failed to send in the last
   hour" (`failedSendsLastHour`). Each links to the kwatch page.
2. **Status tile**: one sentence: "37 issues happening now · 2 urgent",
   "All clear · nothing is failing", "kwatch is loading what it knows", or while kwatch can't be reached "Can't reach kwatch · last
   known at 19:44" (see "When kwatch can't be reached"); the
   tile is a `--failing-bg` panel with a 1px `--line` border, the
   sentence in `--text` and a small marker: red with an urgent issue,
   amber with others, green when clear, with the freshness line (and, on
   a phone, a **Refresh** button beside "Live").
3. **Severity bar**: the active issues by alert level (`counts`) as one
   bar that starts at 0 and spans their total, with a key that gives
   each level's word and number ("Urgent 2 · Needs attention 9 · Low
   26", Quiet only when there are some). A segment or key opens the
   Issues table at that level (`/issues?status=active&severity=page`).
4. **Look at these first**: up to five issues (`lookFirst`: urgent and
   not acknowledged first, then the newest), each with its severity and
   state chips, the title (two lines at most), where, and how long it
   has been going on ("for 40 min", with a bar against the longest of
   the five), "Acknowledged" when someone did. "See all 37 issues →"
   opens the Issues table, and a link opens Activity.

5. **This week**, below a line (`js/ovinsights.js`, `js/trends.js`,
   from `/api/insights` and `/api/trends`), kept short on purpose:
   - four numbers: **Opened** (with "4 more than last week" when the
     previous week is known), **Resolved**, **Paged the on-call
     person** and **Time to resolve** (the median);
   - **Day by day**: a column chart per day: opened (its paged part at
     the bottom) and resolved, each column labeled with its number, 0
     included, all from 0 on one scale, with a key. "Show as a table"
     has the same numbers, with a "Settled before alerting" column when
     there are any (`untold` of the `/api/insights` day when it carries
     one, else of the `/api/trends` day with the same date). A quiet
     week says "A quiet week: nothing opened or resolved.";
   - **Top causes** (`causes`) beside **Where the trouble is** (the
     namespaces with active issues by level, with how many issues each
     opened this week), five rows each, with a link to the rest;
   - **More**, folded: **Kept coming back**, **Started right after a
     change** (`afterChange`) and **Nodes with the most issues**
     (`nodes`), each only when it has something to show.

Cards carry a title only, and a card with nothing to show is left out.
kwatch's own health is on the kwatch page and the change feed on
Activity.

## Issue page

`/issues/<id>` (`js/detail.js`, `incpanel.js`, `incwhy.js`,
`share.js`, `inclinks.js`, `lastlook.js`, and the section modules) puts
the summary card on top, always shown, and everything else in five
tabs. Times are hh:mm; the seconds are in the tooltips.

**The header**: the severity chip ("Was Urgent" once resolved), the
"Settled before alerting" label when it applies, Copy summary and Copy
link, the title, then one line: "shop · deployment/payments-api · for
40 min (since 03:28) · failed again 2 times", or "lasted 12 min,
resolved 14:02" on a resolved issue. When kwatch stopped tracking an
issue while it was still failing, a prominent "Stopped tracking: not
fixed" banner sits above the card and says whether a container still
crashed or it was short of replicas.

**The stepper** (`summary.stage`): Confirming → Happening now → Looks
better → Resolved, the current step marked with "since 03:29"; it says
"Keeps coming back" on a flapping issue and "Stopped tracking" in place
of Resolved. One line under it says what that means ("kwatch is
checking it's real before it alerts anyone.", "It stopped failing 2 Nov,
18:50.", "Settled before alerting — nobody was told."), and while a
timer runs, why it is still open ("Closes after 3 min healthy").

**The notes row**: one muted line of short notes, each at most one line
with an icon and its full text in the tooltip: Caused by (a link to the
issue that causes this one) and "Can't see it right now" (`why.cannotSee`)
come first and stay in full ink; then "Acknowledged 04:00 by
kubectl-annotate" (the note in the tooltip), "Secrets hidden" (a part
shows `[redacted]`), "Moved into" / "Includes earlier issues" links,
"3 things changed since you last looked (12:05)" and "Some text
shortened" (`truncated.texts`). The row hides when there is no note.

**The five summary rows** (from `summary`, see "Issue summary"):

| Row | Shows |
|:--|:--|
| What's wrong | `what`, one plain sentence |
| Why | `why.text`, with the **confidence meter**: three steps and a word, never color alone (High confidence 3 of 3, Likely 2, Possible 1; the tooltip says what it means). Without a cause: "Cause not found yet." and what kwatch checked |
| Proof | up to three pieces of `proof`, strongest first, each with an icon, its kind ("Health check", "Log", "Exit", "Scheduler", "Endpoints", "Change", "Node", "Event"), the short text, "×41" when repeated and the time, then the raw value in monospace with its own Copy (click or Enter shows it all) |
| What to do | the first fix step and its copyable command, then "2 more steps →" (opens them in Details) or the runbook link |
| Who was told | one line per channel, pagers first, each with a mark and a word ("paged · still open", "page failed", "sent", "sending, will retry") and the time; "Every message and why it failed →" opens it in Alerts. Without a message: why nobody was told ("Nobody was told: it settled before alerting.", "Low issues wait for the daily summary." …) |

A row with nothing to say is left out, except What's wrong, Why and Who
was told.

**Tabs** (`KW.ISSUE_TABS`): **Timeline** (what happened, step by step,
with the states strip, and what changed just before), **Evidence**
(failing probe, why it's waiting to start, logs, containers, recent
warnings, node), **Impact** (who it hits, affected objects, traffic,
other issues), **Alerts** (alert status, who was told and what we sent)
and **Details** (why kwatch thinks this, all steps to fix, same error
elsewhere, seen before, more details). Each tab names its count, and an
empty tab says so in one sentence. Keys: `t` `e` `i` `a` `d` pick a tab
from anywhere on the page, `←` `→` (and Home, End) move between tabs
when one is focused. The tab is in the address as a deep link
(`/issues/<id>#evidence`), so a pasted link opens that tab; without one
the page opens the tab last picked in this browser (`localStorage` key
`kwatch.issueTab`), else Timeline. The old `?tab=evidence` link opens
Details.

**The drawer** (see "The preview drawer"): a click on a row of the
Issues table opens the same summary card in a panel beside the table,
with "On the full page: Timeline (1) · Impact (3) …" links instead of
the tabs.

## Releases

There is no Releases page: a release is part of the issue it caused.
`/releases` redirects to `/issues`; `/api/releases` is gone (the
dashboard was only in release candidates). kwatch still watches every
rollout of a Deployment, StatefulSet or DaemonSet and keeps its book of
releases (`dashboard.releases` in the state file) as before; the book
drives the alerts, the undo commands and the two places below.

A release is a change of the pod template (an image, the environment,
resources, probes, config references, volumes, `kubectl rollout
restart`); a replica, pause or label change is none. kwatch's own
Deployment and namespaces it does not watch are left out.

**On the issue** (`js/release.js`, from `release` on
`/api/incidents/{id}`): a row of the summary between the proof and
"What to do", shown only when a release is linked to the issue. Its
label follows the verdict: "Likely caused by a release" for failed,
down and regressed; "A release near this issue" for the others (the
release is then only close in time). It shows the workload (mono), when
it rolled out and how long before or after the issue started, the
verdict as a chip with a sentence, what changed ("Image app: api:1.4 →
api:1.5", "Also changed: environment, probes", the GitOps application or
the field manager, the `kubernetes.io/change-cause` note) and how to
undo it. Pro adds the restarts per hour before and after and the first
fatal line a new container stopped with, only when kept (Community shows
neither, with no empty slot). The undo:

- a workload Argo CD or Flux deploys: "Revert the change in Git: a
  rollback in the cluster would be overwritten by the next sync" (text
  only);
- a Helm release (the `meta.helm.sh/release-name` annotation):
  `helm rollback <release> -n <release namespace>`, the namespace from
  `meta.helm.sh/release-namespace`, else the workload's; a chart label
  alone (`helm template | kubectl apply`) is no release and takes the
  kubectl undo;
- anything else: `kubectl rollout undo <kind>/<name> -n <namespace>`.

A command comes with "Run this in your terminal. kwatch does not change
your cluster." and a Copy button: the dashboard stays read-only. Only
the workload's newest release offers a command: a rollback goes back one
revision from what runs now, so on an older release it would bring back
the newer one. An older release says to undo from the newest.

**On the Overview**: one line when there were releases in the last 24
hours: "4 releases in the last 24 hours, 1 went wrong" ("all fine so
far" when none did). "went wrong" links to the issue (one) or to
`/issues?status=all&release=bad` (several): the issues that releases
which went wrong caused. A release that went wrong without an issue (a
regression: the release watch saw the new version restart far more than
the previous one, but no issue opened) has no issue to link, so the line
lists up to five such releases beneath it with their verdict and time. No
issue is opened for it.

A Deployment stalls only when it misses its progress deadline: a slow
rollout that keeps going is still rolling out. A StatefulSet or
DaemonSet stalls when no replica was updated or got ready for 15
minutes; one that waits for people on purpose (the `OnDelete` strategy,
a StatefulSet partition) never stalls. Without change history (not Pro),
no release shows Pro detail, even one kept from a Pro run.

| Verdict | Means |
|:--|:--|
| Rolling out | the new pods are still replacing the old ones |
| Watching | rolled out; watched for 15 minutes, as the release watch compares a new revision with the previous one |
| Healthy | rolled out and nothing broke while kwatch watched it |
| Regressed | the release watch found the new version restarting far more than the previous one |
| Failed | an issue people were told of started about the workload while it was watched, or the rollout stalled, while the workload still has ready replicas |
| Down | as Failed, with no ready replica |
| Replaced | a newer release came before a verdict |
| Unknown | kwatch saw the release more than 30 minutes after it happened (after a restart, say) |

A release keeps the worst verdict it reached while watched (its rollout
time plus 15 minutes). A healthy release sends no message and nothing
is added to the digest. A release that went wrong is reported by the
issue it causes; when the release is at least the likely cause, its
message offers the same undo (a revert in Git for Argo CD and Flux, a
Helm rollback, or a rollback to the revision before the release). The
book is kept in the state file (`dashboard.releases`), at most 500
releases or 7 days, and in memory without one; it is looked at every 15
seconds.

## Map

`/map` (`js/map.js`, `js/maplayout.js`, `map.css`, from `/api/map`)
draws how objects connect and where it is failing, from relations every
cluster has: an Ingress or HTTPRoute routes to a Service, a Service
sends to the workload whose pods it selects (pods are folded into their
Deployment, StatefulSet, DaemonSet or CronJob, "2/3 ready"), and, where
kwatch knows them (configuration or observed calls), the Services a
workload calls, including those of other namespaces (dashed boxes). It
works without any call data: the drawing then stops at the workloads.

- **One namespace** (default; the one with the most failing objects
  when none is picked) or **What's failing**: every object an open
  issue touches across the watched namespaces, with the objects one
  connection away. Namespaces kwatch does not watch never show.
- **Show nodes** adds the nodes the workloads run on.
- Boxes take the color of the worst open issue touching them (root,
  members or impact): urgent red, needs attention amber, low outlined,
  no open issue plain. A box with an issue opens it; any other says its
  state below the drawing. Boxes are reachable with Tab and open with
  Enter.
- Objects are laid out left to right by how traffic flows (routes,
  Services, workloads, what they call, nodes), ordered in each column
  to keep crossings few, and drawn as SVG with no outside library. The
  drawing scrolls inside its frame.
- **Objects and connections** lists the same objects and edges as text,
  under the drawing; under 700 px wide only the list shows.
- More than 150 objects are not drawn: the page says how many there are
  and asks for a namespace.

## Activity

`/activity` (`js/activity.js`, see "Activity API")
lists what changed recently, newest first: one row per change with the
time, what happened (New, More urgent, Came back, Resolved), the
severity, "Settled before alerting" on a resolve that reached nobody,
the title (two lines at most) and where. "Last hour" / "Last 24 hours"
pick the window (`?since=24h`); one line counts the changes by kind
("6 changes in the last hour: 1 came back, 5 resolved."). With none it
says "Nothing changed in the last hour." in an empty box.

Changes nobody was told about are left out by default: an issue that
settled before alerting (kwatch confirmed it, then it recovered or moved
before a message went out), its resolve, and earlier occurrences no
message reached. A blip while a rollout settles or while kwatch itself
restarts is not news to the reader. "Show 12 changes nobody was told
about" adds them back, each marked "Settled before alerting", and keeps
the choice in the address (`?untold=1`); "Hide changes nobody was told
about" removes them again. The link shows only when there are some.

## kwatch page

`/kwatch` (`js/kwatchpage.js`, `ovcoverage.js`, from `/api/kwatch`,
`/api/status` and `/api/coverage`) answers "is kwatch itself working,
can it see the cluster, and do alerts get out": the same notices as the
Overview plus "kwatch can't read pod logs", one verdict line ("kwatch is
running, but some alerts aren't getting out"), and the cards **kwatch
itself** (running since, last restart and why, last API contact,
messages waiting, restarts in the last 24 hours on a time line) beside
**About this kwatch** (version, plan, last check, scope), **Where alerts
go** (`delivery`, per channel: kind, the last failure or "delivering", a
bar of sent, failed and retrying with the numbers in words, the last
error) beside **Resources** (`resources`: a gauge each for memory "182
MB of 256 MB limit (71%)", CPU "0.04 of 0.5 cores (8%)" and disk "96 MB
of 1 GB volume (9%)" with the state file under it, "State file 38 MB
(cap 512 MB)"; amber from 80%, red from 90%; without a limit the use
and "no limit set", no gauge; a part kwatch can't read is left out),
then across the page **What kwatch is watching** (`coverage`: "233 of
236 kinds of objects watched" with a bar and counts that can't be read,
not read yet and not watched; folded, the gaps and the "Permission to
add" YAML, see "Watch coverage"). Half-width cards come in pairs, so
no row has a gap: one left without its pair takes the whole row. Before
`resources` is known the Resources card is the old **State file** gauge.
Last, only when there are some, **Likely wrong causes (last 7 days)**
(`likelyMisses`, Pro): issues that recovered after something other than
the cause they named changed, each linking to its issue with the cause it
named and what changed instead, and, while the flight recording still
holds it (48 hours), the `kubectl exec … kwatch replay-export --since …
--until …` command to export it (see "Cause self-check" in
docs/production-operations.md).

## Keyboard

The **?** beside the logo opens the help: the colors and alert levels,
the states in `KW.STATE`'s words, "Settled before alerting", and the
keys. It opens above the page and scrolls inside when the window is
short.

| Keys | Does |
|:--|:--|
| `g` then `o` / `i` / `m` / `a` / `k` | Overview / Issues / Map / Activity / kwatch |
| `/` | search the issues |
| `j` `k` | next / previous row, or issue while one is open beside the table |
| `Enter` | open the issue beside the table (Cmd or Ctrl and click: its own page in a new tab) |
| `t` `e` `i` `a` `d` | on an issue page: Timeline / Evidence / Impact / Alerts / Details |
| `←` `→` | move between the issue page's tabs |
| `[` `]` | previous / next issue in your list, on an issue page or beside the table |
| `?`, `Esc` | show or hide the help; `Esc` also closes the issue beside the table |

## Issue summary

`/api/incidents/{id}` carries `summary`, the top of the issue page in
five lines, computed by the server from the other parts of the same
response (before they are cut to the size limit), so the page never
stitches it from twenty fields. Every text is redacted.

| Field | Data |
|:--|:--|
| `what` | one plain sentence of what is wrong, the visible symptom first: a Service of the issue with endpoints and none ready ("Deployment api in shop is not serving traffic: Service api has 0 of 3 endpoints ready."), else a failing probe, readiness first ("… is failing its readiness check, so it gets no traffic."; a liveness probe only when the kubelet killed the container for it, not after the container crashed by itself), else the failure family ("keeps crashing (12 restarts)", "is stuck pending: no node can run it (3 pods waiting)", "can't pull its image" …), else the headline. A resolved issue is in the past tense ("was stuck pending."). At most 500 characters |
| `why` | `text`, the cause sentence (empty without a cause), `confidence` (`high` at a score of 0.70 or more with no rival cause, `likely` from 0.50, else `possible`; empty without a cause) and `causeKnown` |
| `proof` | up to 3 pieces of evidence, at most one per `kind`, in this order of strength: `probe` (the most often failing probe with the kubelet's message), `log` (the last captured log line that reports an error, panic, failure, refusal or timeout, or that says the application is unhealthy, not ready or answered a 5xx status; this includes the line a running container logged while its probe got an HTTP error status, which investigation captures), `exit` (how the most restarted container last ended: reason and exit code, from the live containers or else the latest break), `scheduler` (why pods wait for a node), `endpoints` (the Service or Ingress that lost the most ready endpoints), `change` (the change the likely cause blames), `node` (a node not ready or with an unhealthy condition) and `event` (the Warning event repeated most). Each item is `{kind, text, detail, count, at}`: `text` short and plain, `detail` the raw value (the log line, the probe message), at most 300 characters. An item whose raw value (case and spacing aside) an earlier one already showed is skipped for the next of its kind: a probe message repeated as a Warning event is shown once. A container that exited by itself with an error code ranks `log` and `exit` above a liveness probe (readiness keeps its place): the probe failing while it restarts is a symptom. When the kubelet killed it for the probe (a Warning event "failed liveness probe, will be restarted"), the liveness probe leads and an exit 137 or 143 reads "restarted by kubelet after failing its liveness check", unless a captured log line shows the program had already stopped with a fatal error at start (an unhandled exception, a panic, an invalid or missing setting: `explain.FatalStartLine`): that line then leads, before the probe |
| `next` | the first fix step, `{text, command, more}`, `more` counting the steps after it; absent without steps |
| `told` | per channel the incident's messages reached, `{name, provider, kind, state, at}`: `kind` is `pager` (the send trail marks it, or a pager row exists), `chat`, `email` or `other`; `state` is the pager's own alert state for a pager (`open`, `closed`, `failed`, `pending`), else the latest send's (`sent`, `failed`, `pending` while it is retried); pagers first, then by name |
| `stage` | for a stepper: `step` is `confirming` (settling), `happening` (open or flapping), `recovering` or `resolved`, `since` the latest transition into that state (else the opened, recovering-since or resolved time), `flapping` and `stoppedTracking` |

The summary is cut last when the detail is over the size limit: first
channels down to 3, its texts to 100 characters, proof down to 1, then
`told`, `next` and `proof` go (`truncated.summaryTold`,
`summaryNext`, `summaryProof`) and the texts are cut to 40. The step's
command is never cut.

## Issue page context

`/api/incidents/{id}` carries these parts besides the incident itself.
Each is omitted when there is nothing to show, bounded where it is built,
cut further by the response size limit (64 KiB) in a fixed order, and
counted in `truncated` when entries are left out. `truncated` has one
count for every list above (`transitions`, `snapshots`, `snapshotRows`,
`phases`, `fixSteps`, `causedBy`, `causing`, `pager`, `sent`,
`movedFrom`, `changedBeforeRecovery`, `changes`, `containers` ...) and
`texts`, how many long texts were shortened (titles, cause, fix steps,
reasons, sent texts, change values ...). When `texts` is more than 0 the
notes row of the summary card says "Some text shortened".

| Field | Section | Source | Bound |
|:--|:--|:--|:--|
| `changes`, `fixedBy`, `fix` | What changed just before | the changes the incident stored while it was live (first), merged with the inventory's change history around the root (what it uses, routes to or scales it, its node), from 30 minutes before the failure began until it resolved; `fixedBy` is the change kwatch judged fixed it, and a change with `cause` is the one the likely cause blames (the page marks it "Likely cause" and names it in the summary card). Each change carries `fields` (`path`, `before`, `after`) for a diff: at most 6, values redacted and cut to 160 bytes, and the data of a Secret or ConfigMap names its key only (`after` is `added`, `changed` or `removed`) | 8, before first, then during |
| `seenBefore` | Seen before | the incident's own history of earlier occurrences of the same root (kwatch keeps the last 30) | 30 |
| `snapshot`, `snapshots` | When it broke | the failing containers (not ready, restarted, or waiting or ended for a reason) of the incident's pods, stored when it opened (`reason: "first break"`) and replaced each time it got worse (`"got worse"`: its tier rose or it reopened); same row fields as `containers`, as they were then. `snapshot` is the latest of `snapshots` (oldest first) | 2 snapshots, 10 containers each |
| `transitions` | the states strip | every state and tier change: `at`, `kind` (`opened`, `announced`, `recovering`, `failing again`, `flapping`, `reopened`, `resolved`, `state changed`, `tier raised`, `tier lowered`), `from`, `to` (state names, or tier names for a tier change) | 50 newest |
| `why` | Why this level, why still open | `alertLevelReason`: why the incident is at its alert level, in plain words (a page rule, a known or routine failure, a configured override, a level kept from earlier …); `stillOpenReason`, e.g. "Looks better; closes after 3m healthy, 1m20s so far", "Confirming: making sure it still fails before alerting, 40s left", "Flapping: waiting for 30m stable, 5m so far"; `holdTotal` and `holdElapsed` in seconds; `cannotSee`, true when kwatch can't see the object right now (the page says so and shows no countdown). Timers come from kwatch's latest tick; a resolved incident has only `alertLevelReason` | |
| `ack` | Acknowledged | `note` (the annotation's text, redacted), `since` (when kwatch saw it or its note change), `by` (the tool that wrote it: the field manager the object's managed fields name for the `kwatch.dev/ack` key, such as `kubectl-annotate`; absent when they name none) and `on` (the object that carries it) | |
| `causedBy`, `causing` | Caused by, Other issues | the other open incidents this one is caused by and the ones it causes, as the reasoning ties them (a failure chain that goes on past its limit into the other's failures, or a cause rooted at the other's root): `id`, `title` (the headline of its last message, else its root), `severity`, `state` | 5 each |
| `blastRadius` | Who it hits | one line from the impact and the failing pods, such as "Affects 2 services, 1 ingress and 3 of 4 pods of payments"; once nothing fails (or after the resolve), the widest it reached | |
| `phases` | the states strip | that history and the incident's open, recovering and resolve times | the last 24 hours |
| `containers` | Containers | the live inventory: the containers of the root's and failing objects' pods, with requests, limits and kubelet usage | 10, most restarted first |
| `affected` | Affected objects | root (`root`, shown as "this issue's object"), the object the likely cause blames when it is not the root (the object of its blamed change, `status: cause`, shown as "Likely cause"), merged roots, failing objects (a container counts as its pod) and impact, with `kubectl` commands | 20 |
| `deliveries` | Who was told | delivery's send trail per incident, saved in the state file (provider, kind, placement, result, retries, scrubbed error, `reason`, the failure in plain words such as "token refused (401)" or "rejected: message too long", and `fallbackFor`, the provider a fallback stood in for; both named as products are, "PagerDuty"); rows read "Slack · update sent" | 16: the newest and the first accepted |
| `warnings` | Recent warnings | Kubernetes Warning events (inventory notes) on the root, merged roots, failing objects, impact, pods and their containers, from 30 minutes before it opened; folded by reason and redacted message, with count, last seen and the object | 10 newest |
| `traffic` | Traffic impact | the Services among the incident's objects or selecting / routed to its pods, and the Ingresses routing to them: ready vs total endpoints summed over their EndpointSlices now; a Service is left out when no EndpointSlice is known in its namespace | 8, not-ready first |
| `nodes` | Node | the root node, or the nodes the incident's pods run on: zone, pool, instance type, spot / on-demand (`karpenter.sh/capacity-type`, `eks.amazonaws.com/capacityType`, `cloud.google.com/gke-spot`, `cloud.google.com/gke-preemptible`, `kubernetes.azure.com/scalesetpriority`), age, unhealthy conditions (redacted), the incident's pods on it and other pods on it not ready | 3 nodes, 6 conditions, 256 pods read per node |
| `probes` | Failing probe | the containers of liveness / readiness / startup probe findings: check (`http /path :port`, `tcp :port`, `grpc :port`, `exec <program>`; exec arguments are never recorded), delay, timeout, period, failure threshold, and the latest `Unhealthy` event (redacted) | 4 |
| `timing` | "Alert status" | `startedAt` (the earliest start of its failure findings, as Kubernetes reported it, never after it opened), `noticedAt` (when kwatch opened it), `toldAt` and `toldVia` (the first message a channel accepted at or after the open, from the delivery trail) | |
| `sameTime` | Other issues ("Started at the same time") | the other incidents that opened within 5 minutes of this one (`id`, `title`, `severity`, `state`, `namespace`, `startedAt`, `relation`): `same node` (a node they share: a root, merged root or the node their pods run on; checked for the 64 nearest), then `same namespace`, then `cluster`, nearest start first; active quiet issues are left out | 8 |
| `usageHours` | Containers ("Memory, last 24 hours", or CPU) | only as evidence: memory when a finding is about memory (OOMKilled, memory high) or a container was killed for it or uses 90 % or more of its limit; CPU only for a CPU throttling finding. Per container: `metric` (`memory` in bytes, `cpu` in millicores), `limit`, and `peaks` (`start`, `value`), the kubelet stats poller's hourly peaks, oldest first | 3 histories, 24 peaks each |
| `pager` | Alert status | one row per pager that holds this incident: `provider`, `name`, `state` (`open`: paged and not resolved there, `closed`, `failed`: refused for good, `pending`: still retried), `pagedAt`, `closedAt` and the last failure's plain `reason`; drawn as lines such as "Paged PagerDuty at 12:31 · still open on the pager" | |
| `sent` | What we sent | the latest message each channel accepted: `provider`, `name`, `at`, `kind` and `text`, the plain text kwatch composed before the channel's own formatting, at most 2 KiB (`cut` says it was shortened); sorted by provider | |
| `moved` | "Moved into", "Includes earlier issues" | `into`, the issue this one was moved into, and `from`, the earlier issues moved into it (each `id` and `title`); only issues kwatch still keeps | |
| `logsNotReadable` | "Crash output not shown" | true when the log section is empty because kwatch may not read pod logs | |
| `changedBeforeRecovery` | Changed just before it recovered | on a resolved issue only: the changes made shortly before it stopped failing, newest first, same shape as `changes`. It never says they were the fix | 2 |
| `title`, `untold` | the issue's title and its "Settled before alerting" mark | `title` (also on list items) is the incident's problem title: the lead of its announcement, written when it opens and when the problem changes, never an update's, a recovery's or a move's wording, at most 140 characters; the root's key only when there is none. `untold` (also on list items, and part of the row's `rev`) marks an issue no message reached: it settled, recovered or moved before alerting | |
| `stoppedTracking`, `stillCrashing` | the "Stopped tracking" banner | `stoppedTracking` (also on list items, and part of the row's `rev`) is set on a resolved issue that kwatch stopped following while it was still failing; `stillCrashing` says a container still crashed (else it was short of replicas) | |
| `waiting` | Why it's waiting to start | the incident's pods that are Pending with `PodScheduled=False`: the newest `FailedScheduling` event, else the condition message (redacted), parsed into plain-words reasons with node counts, plus the raw message | 1 message, 8 reasons |

Fields another part of kwatch may add are drawn when present and
ignored when absent: `causedBy` (one issue or a list, each an `id` and
`title`) as "Caused by: …" first in the notes row of the summary card,
`causing` as "Also causing" in Impact's "Other issues", and
`blastRadius` (a sentence, or counts) as "Who it hits" in Impact.

**Alert status** (`js/incwhy.js`) is the first section of the Alerts
tab; why it is still open also shows under the stepper. It says why it is still open in plain words ("Looks better ·
closes after 3 min healthy · 1 min 20 s so far") with a bar that fills
each second between reads while `holdTotal` and `holdElapsed` run (a
settling issue counts its "40 s left" down); "Why you got this alert:"
with `alertLevelReason`; "Started 12:30 · kwatch noticed 12:30 ·
told Slack 12:31" from `timing` (each with a tooltip that has the
seconds); one line for each pager in `pager` ("Paged PagerDuty at 12:31 ·
still open on the pager", or "page failed: token refused (401)"); and the
acknowledgement, "Acknowledged 12:40 by kubectl-annotate · “note”", or,
when nobody has, the command that sets it
(`kubectl -n <ns> annotate <kind>/<name> kwatch.dev/ack="looking into
it"`) and the one that removes it (`kwatch.dev/ack-`): the dashboard
only reads. The timers are left out of the comparison that decides
whether the issue changed, so a ticking timer never shows "This issue
changed"; a new read moves only the timer.

When `why.cannotSee` is set, the notes row says "Can't see it right
now" (the tooltip: "kwatch can't see this right now, so it stays open
until it can check again.") and no countdown runs; an acknowledgement
is also a note there, "Acknowledged 12:40 by kubectl-annotate", with the
note in its tooltip.

The states strip (`js/transitions.js`, in the Timeline tab) is drawn from `transitions` when
there are any (one segment per state, so every flap shows; alert level
changes are marks above it), else from `phases`. Transitions are also
entries of "What happened, step by step", except the opening and those a
timeline note already tells within a minute. "Containers" offers "At
first break (12:30)", "When it got worse (12:41)" and "Now" when
`snapshots` exist. Each change with `fields` has "Show what changed": the
path, before → after, or for Secret and ConfigMap data only that the key
was added, changed or removed.

**Changed since you last looked** (`js/lastlook.js`, `localStorage`
key `kwatch.looked`, this browser only, at most 200 issues, forgotten
after 8 days): the browser keeps when it last showed each issue. On the
next visit the notes row says "3 things changed since you last
looked (12:05)" and the newer timeline entries, transitions, changes and
warnings carry a "New" mark. The first visit marks nothing; without
`localStorage` nothing is marked.

The header's **Copy summary** button copies, built in the browser
from this response: the title, severity in plain words, state when not
"happening now", where, since (UTC), likely cause, the first fix step and
its command, and a link to the page.

Small things on the issue page. A secret kwatch hid (`[redacted]`) in a
log line, a timeline entry or a sent text shows as a small "hidden" tag.
When `logsNotReadable` is set, the Logs section says "Crash output not
shown: kwatch isn't allowed to read pod logs." with a link to the
coverage panel that says how to allow it. A resolved issue that kwatch
gave up on shows a "Stopped tracking" banner that says it is not fixed
and whether a container still crashed or it was short of replicas. "What
we sent" lists, folded, the text each channel accepted, with its time
and "(shortened)" when it was cut.

Gating: the dashboard is Pro. Within it, `changes` needs the change
analysis (`analysis.changes`), and usage readings exist only where the
kubelet stats poller runs (`core.nodes.usage`); without them those parts
are simply absent. Containers and usage are the inventory as it is now,
not as it was when the issue started (the `snapshot` keeps them as they
were when it broke); the change history reaches back about two hours,
while the changes an incident stored (at most 12, looked for once a
minute while it is live) are kept with it in the state file, as are its
snapshots, transitions and acknowledgement. The send trail survives a
restart too (it is kept in the state file for the 512 most recent
incidents). An incident saved by an older kwatch has none of these until
they happen again.
Usage changes on every read, so a new reading redraws only the Containers
section, not the open sections around it.

`capturedLogLines` are the output lines and quoted facts the incident's
messages carried, at most 25, kept with the incident (and in the state
file). When more arrive than fit, kwatch keeps the start of the trouble
and the most recent lines rather than only the newest: the first line
that names a failure and is not a stack frame (the same rule that picks
the error line a message quotes) and up to 8 lines from it, then a line
`… N lines skipped …` counting what was left out, then the newest lines.
Lines from before that first error are dropped. Later lines move past
the marker and its count grows, so the start stays however long the
incident runs. Without any error line the newest 25 are kept.

The container usage history behind the Containers section (the 24-hour
memory and CPU peaks, and the run before the last restart) is saved in
the state file and survives a restart of kwatch; see "Kubelet stats" in
`production-operations.md`.

## Issues table

`/issues` (`js/issues.js`, with `issuetable.js`, `colmenu.js`,
`columns.js`, `issuesfilters.js`, `filterchips.js`, `dropdown.js`,
`selectmenu.js`, `issueslive.js`, `quickviews.js`, `viewopts.js` and
`prefs.js`, `pager.js`, `drawer.js`, `place.js`, `highlight.js`) is one
table for every issue, one row per
issue. The server filters, sorts and pages it, so it behaves the same
with ten issues or ten thousand.

**Status.** A switch at the top: *Active* (the default), *Resolved* or
*All*, each with its count; a line under it says what each means (no
tooltip). Next to it, **Filters** opens the other pickers (kind, state,
acknowledged, stopped tracking, start time, resolved within); the
namespace picker and the search stay in view. Resolved and All show "Resolved within"
(last 24 hours, 3 days, or 7 days: kwatch keeps resolved issues a
week); it narrows resolved issues only, never active ones.

**Columns.** By default: the issue's title first and flexible (two lines
at most, cut with "…"; the tooltip has the whole title and the likely
cause; after it, small marks for how often it failed again, `× 3`, New /
Changed since your last visit, the muted labels "Settled before
alerting" (see "Layout and reading order") and "Stopped tracking"),
the severity badge, where (one mono
cell: the namespace, or "whole cluster", then the object: `payments ·
deployment/payments-api`; it replaces the old Namespace and Object
columns, and a view that kept them keeps Where), status, when (on Active
"Going on": "for 3 h", or "now" under a minute, the exact start in
the tooltip; on Resolved "Resolved": "3 h ago"; "Going on or
resolved" on All). Acknowledged, start time and duration can be added in
the Columns panel. The Resolved list has no Status column
(every row is resolved). Status is in plain words:

| API state | Shown as | Tooltip |
|:--|:--|:--|
| `open` | Happening now | This is failing right now. |
| `settling` | Confirming | kwatch is checking before it alerts. |
| `flapping` | Keeps coming back | It keeps failing and recovering. |
| `recovering` | Looks better | It stopped failing; kwatch waits a little to be sure. |
| `resolved` | Resolved | It stopped failing and stayed fixed. |

A click anywhere on a row, or Enter on its title, opens the issue in a
drawer on the right, over the table (see "The preview drawer"); a row
never expands or collapses. The title is a real link to `/issues/<id>`,
and Cmd- or Ctrl-click, Shift-click or a middle click anywhere on the
row opens the issue page in a new tab. A click that ends a text
selection opens nothing. `j` / `k` move between rows; the row with the
keyboard focus is marked as a whole, and the focus stays on the same
row, heading, pager button or filter chip (by its key) when a live
update, a new page, a sort or a removed chip draws them again (a
removed chip hands it to its neighbour). A "Skip to issues" link,
first on the page for the keyboard, goes to the first row. Below 1100
px wide the Where column hides and the sidebar and page margins
narrow; below 900 px each row becomes a card
(the column name above each value, empty values left out), so the page
never scrolls sideways; the order is then picked with the "Sort"
picker above the table, since the column headers are hidden. The
title column keeps at least 14rem; the other columns have fixed widths
(so they stay put between views) unless dragged.

**Sorting.** A click on a column heading sorts by it on the server; a
second click reverses it. Each heading's menu (see "Columns you choose")
also has the two directions in the column's own words: "A→Z" and
"Z→A" for text, "Oldest first" and "Newest first" for times, "Least
urgent first" and "Most urgent first" for severity, "Shortest first"
and "Longest first" for duration. The heading carries `aria-sort` and
an arrow. Severity, the times, duration and repeats start high to low;
text starts A to Z. Without a sort the order is kwatch's, "Most
urgent, then newest": on Active, by severity (Urgent, Needs
attention, Low, Quiet), then the newest started; otherwise the latest
activity first. Within one severity, newest first holds whichever way
severity is sorted. Ties always fall back to the issue ID, so a page
never changes order on its own.

**Paging.** "Showing 51–100 of 4,812" under the table, with First,
Previous, the page numbers (`1 … 4 5 6 … 97`), Next, Last and the rows
per page (25, 50 or 100). The page is in the address (`page=3`) and
goes back to 1 when a filter, the order or the page size changes. A
page is always full: a list item is cut to fit, so 100 rows always fit
a response. A page past the last one shows "Nothing on this page" with
a link to the first. The pager is hidden while there is only one page.

**Columns you choose** (`js/columns.js`, `js/colmenu.js`). The
**Columns** button at the top right of the table opens a panel with
every column: a checkbox shows or hides it, the handle drags it to
another place (the up and down buttons do the same from the keyboard),
the presets *Compact* (issue, severity, where, when),
*Standard* (the default) and *Everything* set them in one click, and
*Reset columns* goes back to the standard set, order and widths.
Severity and Issue can't be hidden; the issue stays first, and its row
carries the severity color strip. Each heading also has a menu (its ⋮ button,
shown on hover or focus, or a right-click on the heading): the two
sort directions, Hide column, Move left, Move right. It stays open
while the table updates. A
heading's right edge drags to resize the column (56 to 640 px; arrow
keys on the focused edge resize by 16 px; a double-click fits it to
what the page shows). Columns nobody resized fit their content (the
short ones) or take a share of the table (namespace, object); the
title takes the rest. When the chosen columns can't fit, the table
scrolls sideways inside its frame, never the page. Over 900 px wide
the page is one screen high: the head and filters stay put and only the
table scrolls, its headings on top (`KW.scrollBox` in `js/core.js`;
the kept place, the live bar and `j`/`k` follow the table's scroll).
Narrower, the page scrolls as a whole. Columns, order and widths are kept in
this browser (`cols`, `order`, `widths` in `kwatch.view`) and in each
saved view. Below 700 px, where rows are cards, the same columns are
the lines of each card.

**Filters and counts.** The search box (with the shortcuts below), the
multi-select pickers **Namespace**, **Kind**, **State**,
**Acknowledged** and **Stopped tracking** (shown on Resolved and All,
and on Active only while a value is picked: only resolved issues stop
being tracked), the **Started** picker (one choice: any time, today,
over a day ago), on Resolved and All the **Resolved within** picker,
and the severity chips. Every filter that is a set of values takes
several: values of one filter are OR-ed, filters are AND-ed. The
severity chips are toggles; each one on shows a check mark, and
several can be on at once; a level with no issues is hidden unless it
is on. A picker's button reads "All namespaces" with nothing picked
and "Namespace" with how many values are picked otherwise (its tooltip
lists them); the values show once, as chips under the bar. Its panel
(`js/dropdown.js`) has a search box for long lists, a checkbox per
value with how many issues it would show, *Select all* (what the
search shows) and *Clear*; it applies each change at once. Keys: Down
opens it, the arrows, Home and End move, Space or Enter toggles, Tab
stays inside it, Esc (or a click outside) closes it and returns to the
button. The state picker offers Happening now, Confirming, Keeps
coming back and Looks better, plus Resolved on All (on Resolved it is
hidden): the Active / Resolved / All switch stays the quick top-level
choice. Namespaces come from the namespace facet (the 50 busiest, with
counts) and kinds from the kind facet (the 20 busiest); values picked
stay listed. At most 100 values are drawn at once; the search finds
the rest. The counts are over the
whole filtered list, each counted without its own filter. Quiet
appears when there are quiet issues.

The values of the pickers in use show as chips under the bar
("Namespace: team-1 ×", "Acknowledged: No ×", in the pickers' own
words), after how many issues match; × removes that one value and
*Clear all* every filter (status, order and page size stay). The
search text, the severity chips and the start time show in their own
controls, so each filter shows once. Typing waits 300 ms before it
asks the server, a newer request cancels an older one, and an answer
for an older view is ignored; the box keeps what you type, spaces
included, while the list catches up.

**Dropdowns** (`js/selectmenu.js`). Every single-choice picker (start
time, resolved within, the sort picker under 700 px, rows per page,
the View options pickers, the notifications level, the log container)
is the site's own dropdown, drawn like the multi-select ones: body
font, input radius and border, a check on the chosen value, the card
border, light and dark. The native `<select>` stays in the
page, hidden, and keeps the value, so code and the address read and
set it as before; a button with a listbox (arrows, Enter or Space,
Esc and Tab close and return focus) replaces its look. A select added
to the page later is picked up by itself.

**Quick views** are one-click presets above the table: *New today*,
*Going on over a day*, *Recently resolved* (resolved in the last 24
hours) and *Stopped tracking* (resolved issues kwatch gave up following
while they were still failing); the Urgent severity chip shows only urgent issues. They only
set filters, so they show in the address. The view shown is
highlighted; clicking it again (or *Clear all*) shows every active
issue again. Your saved views are chips beside them; a saved view
that changes nothing (no filter, the default status and order) is
never shown as the one in use.

**Search shortcuts** (`js/searchq.js`). The search box takes plain
words (each must be found: in the title, namespace, object, ID or
cause) and these shortcuts; "Search shortcuts" under the box lists
them:

| Shortcut | Keeps | Applied by |
|:--|:--|:--|
| `ns:staging` | that namespace (repeat for several) | API `namespace` |
| `kind:deployment` | that kind of root object, any case, dots and digits as in `?kind=` | API `kind` |
| `is:urgent`, `is:attention`, `is:low`, `is:quiet` | that alert level | API `severity` |
| `state:open`, `settling`, `flapping`, `recovering` | that state | API `state` |
| `state:resolved` | resolved ones, on All | API `state` |
| `age:>1d`, `age:<2h` | started over / within that long ago (`m`, `h`, `d`, `w`) | API `startedBefore` / `startedAfter` |
| `is:acked`, `-is:acked` | acknowledged, or not | API `acked` |
| `is:stopped`, `-is:stopped` | stopped tracking, or not | API `stopped` |
| `is:new`, `is:changed` | marked New / Changed since your last visit | the page (this page only) |
| `"out of memory"` | those words together | API `q` |
| `-word`, `-"two words"` | issues that don't mention it | API `exclude` |

A shortcut combines with the pickers: both narrow the list. An unknown
prefix (`foo:bar`) or value (`state:bogus`) is searched as plain text.
`is:new` and `is:changed` are the only filters the server can't apply,
because what you have seen lives in your browser; they filter the page
shown and the table says so.

**New since your last visit** (`js/visits.js`, `localStorage` key
`kwatch.visits`, this browser only; the page keeps only the issues of
the list shown last). For each issue the browser
remembers the severity and repeat count it last showed you. An active
issue is marked **New** when it started after this browser began
keeping track (or after the last "Mark all seen") and you haven't
opened it, and **Changed** when it is more urgent than when you last
saw it, or failed again since (the mark's tooltip says which); the mark
sits after the title. The line under the quick views counts the marks
on the page shown; opening an issue (its page or the drawer) marks it
seen and *Mark all seen* marks the page. The first visit
marks nothing, and issues gone for 8 days are forgotten.

**The preview drawer** (`js/drawer.js`, `js/incpanel.js`). The drawer
shows the issue page's summary card (header, stepper, notes row and the
five summary rows): `detail.js` has one renderer (`KW.renderIncident`)
and `incpanel.js` loads and refreshes an issue for both, so the page and
the drawer never differ. In place of the tabs it has "On the full page:
Timeline (1) · Impact (3) …", each opening the issue page at that tab. Its bar has
Previous / Next (also `j` / `k` and `[` / `]`) through the rows of the
table page shown ("3 of 50 on this page"; at the end of the page they
stop, the pager moves on), "Open full page" (a real link to
`/issues/<id>`) and Close (×, Esc, or a click beside the drawer). The
table keeps its filters, page, scroll and rows underneath. The address
says which issue is open, `/issues?status=active&open=<id>`, so the
link can be shared and opens the drawer; opening adds a history entry,
so Back closes it, and Previous / Next replace it. Focus stays inside
the drawer while it is open and goes back to the issue's row when it
closes. Under 900 px wide the drawer covers the screen. The table's
live updates keep running behind it (rows update in place, a new order
waits for "Show"), and the drawer's issue reads again on its own, every 60 s
while the live stream is connected (a change to it is read sooner) and
every 30 s without the stream.

**Not redrawn while you read.** The issue page and the drawer redraw
nothing while a section is open or text in them is selected: a bar
says "This issue changed. Show the update". The table does not update
its cells while text in it is selected.

**Back to the same place** (`js/place.js`, `sessionStorage` key
`kwatch.place`, this tab only). When the table is left, its address,
scroll position and the row last focused or opened are kept. Coming
back to the same address (Back from an issue page, its "← Issues"
link, or a reload) scrolls there again and focuses that row. Without
`sessionStorage` it works until a reload.

**Search matches** (`js/highlight.js`). When the search has plain words
or quoted phrases (not shortcuts like `ns:` or `-word`), each match is
marked in the table's issue, namespace and object cells and in the
drawer's title, ignoring case. The marks are built as text nodes and
`<mark>` elements and found with `indexOf`, so any character is safe.
A long title whose first match would be cut off starts with "…" a
little before it, so the match shows; a row that matched only by its
likely cause says "in the cause" after the title.

**Previous / Next on the issue page** (`js/incnav.js`). When the table draws, it
remembers its address, its page and its issues in the order shown
(`sessionStorage` key `kwatch.listnav`, this tab only). An issue opened
from it shows "← Previous · 26 of 4,812 · Next →" above the issue and
walks the same filtered, sorted order with the buttons or
`[` / `]`; at the end of a page it reads the next (or previous) page
from the API and goes on (only the latest such read opens an issue,
and only while the reader is still on the issue it started from; a
failed read leaves them there). A click with Cmd, Ctrl, Shift or Alt
is left to the browser. Its back link returns to the exact table,
page included. An issue that isn't in the list, or a tab without one,
shows no bar; without `sessionStorage` it works until a reload.

While the drawer is open it takes every key, wherever the focus is: Esc
closes it, `j` / `k` and `[` / `]` move, Tab stays inside, and `/` and
`?` do nothing (they would act on the page behind it). When a redraw
removes the focused element, focus goes back to the drawer. Opening an
issue in it, or moving to one, marks it seen like opening its page
(`KW.visits.opened`). An answer for an older read never replaces a
newer one.

**Keyboard.** `/` searches, `j` / `k` move down and up the rows (or to
the next and previous issue in the open drawer), `[` / `]` move through
your list on an issue page and in the drawer, Enter opens the issue in
the drawer and Esc closes it. `?` opens the help beside the logo, which
lists every key (see "Keyboard" above); Esc closes it.

### The address

Everything that shapes the table is in the page address
(`js/listquery.js`), so a view can be bookmarked or pasted and opens
the same way for anyone. A link always wins over this browser's
defaults and saved views.

| Query | Meaning | Values |
|:--|:--|:--|
| `status` | which issues (always written) | `active`, `resolved`, `all` |
| `q` | search text with shortcuts, at most 200 characters | any text |
| `severity` | alert level, repeatable | `page`, `notify`, `digest`, `silent` |
| `namespace` | namespace, repeatable | a Kubernetes namespace name |
| `kind` | kind of root object, repeatable | lower case: `deployment`, `node` ... |
| `state` | state, repeatable; Active and All | `open`, `settling`, `flapping`, `recovering`; `resolved` on All |
| `acked` | acknowledged, repeatable | `true`, `false` |
| `stopped` | kwatch stopped tracking it, repeatable | `true`, `false` |
| `age` | when it started | `today`, `day` (over a day ago) |
| `since` | Resolved and All: resolved within | `24h`, `72h` (default: 7 days) |
| `sort`, `dir` | order | `severity`, `title`, `namespace`, `object`, `status`, `started`, `last`, `duration`, `repeats`, `acked`; `asc` / `desc` (the old `sort=recent` is `last`) |
| `page`, `pageSize` | the page | from 1; `25`, `50` (default), `100` |
| `like` | issues with the same error as this one (a chip "Similar to: <title> ×") | an issue ID |
| `node` | issues on that node, repeatable (a chip "Node: <name> ×"; the Overview's "Nodes with the most issues" links here) | a node name |
| `mode` | issues of that kind of failure, repeatable (a chip "Failure: Image pull ×"; the Overview's "Top causes" links here) | a failure family: `ImagePull`, `CrashLoop`, `OOMKilled` ... |
| `open` | the issue shown in the drawer; not a filter, never sent to the API | an issue ID |

A repeatable filter is written once per value
(`namespace=team-1&namespace=team-2`); the old forms, a single value or
comma-separated values (`namespace=a,b`), still open the same view and
are written back in the new form. Old links keep opening: `group` is
ignored, `offset` and `limit` become the page and its size, and
`sort=recent` is `last`. A value the page doesn't know is
dropped from the address and never sent to the API, so a mistyped link
shows the table instead of an error. A click or a choice adds a browser history entry, so Back undoes
it; typing in the search box replaces the current entry instead.

### Live updates on a busy cluster

The table reads the list when the live stream (see "Live push") says
incidents changed, at most once every 5 s, and otherwise every 60 s;
without the stream it reads it every 20 s while the tab is shown (not
while it is hidden). Each read sends the list's `ETag` back: when
nothing changed the server answers `304` with no body. When the list
did change:

- rows already shown update their cells in place (a thin mark shows
  which), and a row that left the list is dimmed and struck through;
- new rows and a new order are drawn at once only when you are on the
  first page, at the top, with no row focused and haven't touched the
  page for 10 seconds;
- otherwise a bar at the bottom of the screen (never over the table
  heading) says what changed, "2 new urgent · 3 new · 12 changed ·
  Show"; Show draws the new list. Nothing moves under the reader until
  then.

The counts, the chips and "Showing 1–50 of 4,812" always update at
once. While the drawer or a column menu is open, or text in the table
is selected, a new order waits for "Show" too. There is no pause: the rules above already keep rows still while
someone reads, and the freshness line ("Live · updated 5 s ago") says
when the list was last read.

Worst case, thousands of issues that change every few seconds: the
server builds the list in about 10 ms for 5,000 issues, a page is at
most 100 rows and 64 KiB, the browser draws only that page, every poll
of an unchanged list is a `304`, and the reader decides when the rows
move.

### View options and saved views

**View options** (the button beside Columns, above the table) sets,
for this browser: density (comfortable or compact), rows per page,
what a bare `/issues` opens with (status and order), text size and
high contrast. Local or UTC times are only in the sidebar.

**Text size and contrast** apply to every page. Text size is Small,
Normal, Large or Extra large (`text`: `s`, `m`, `l`, `xl` in
`kwatch.view`): the root font size is 87.5 %, 100 %, 112.5 % or 125 %
of the browser's, and every font size and the widths that hold text
(sidebar, popovers, drawer) are in `rem`, so the table, drawer, chips
and dropdowns grow together. High contrast (`contrast: "more"`) uses
stronger text and border colors in light and dark, no faded or muted
text and thicker focus rings (`look.css`); its text is at least 7:1 and
its borders 5:1 against the page and surfaces (WCAG AA, mostly AAA).
`js/look.js` is the first script of the page and is not deferred: it
reads `kwatch.view` and sets `data-text` and `data-contrast` on
`<html>` before anything is drawn (the CSP allows it: it is a file,
not inline code). Reset to defaults resets them too. The columns
live in one place, the Columns panel; View options links to it.

**Saved views.** "Save this view" keeps the current filters, order
and columns (which show, their order and widths) under a
name (up to 20). Saved views show as chips
beside the quick views; in View options each can be opened, renamed,
deleted or marked "Open by default", which `/issues` without a query
then opens. When the browser's storage is full, saving says "Couldn't
save: browser storage is full." A link with a query always wins. **Reset to defaults** puts
the view options back (Local times included) and unmarks the default
view; saved views stay.

All of this, and the Local / UTC choice, is kept in `localStorage` (key
`kwatch.view`; the old `kwatch.tz` is still read) and read on every
page: the Overview, the table, the issue page and the trends' `tz`
(notifications name no times). Not cookies: the dashboard keeps no
per-person state on the server, and these settings never need to reach
it, so a cookie would only add bytes to every request. Without
`localStorage` (private mode, blocked storage) the options work until
the page is reloaded.

### The list API

`GET /api/incidents` serves the table (and the Overview, the badge and
notifications). Repeated keys are OR-ed, different keys AND-ed:

| Parameter | Meaning |
|:--|:--|
| `status` | `active` (default), `resolved` or `all` |
| `severity` | `page`, `notify`, `digest`, `silent`; repeatable |
| `state` | `open`, `settling`, `flapping`, `recovering`, `resolved`; repeatable |
| `namespace`, `kind` | repeatable; `kind` is the root's kind, any case |
| `acked` | `true` or `false` |
| `stopped` | `true` or `false`: resolved issues kwatch stopped tracking while they were still failing (`stoppedTracking` on the item) |
| `q`, `exclude` | search terms, repeatable: every `q` must match, no `exclude` may (at most 10 terms and 200 characters in all; more answer `400` "Too many search words: 10 is the limit.") |
| `since` | resolved within a Go duration (`24h`); bounds resolved issues only |
| `startedAfter`, `startedBefore` | started within / before a Go duration back from now (`90m`, `24h`) |
| `sort`, `dir` | `severity`, `title`, `namespace`, `object`, `status`, `started`, `last`, `duration`, `repeats` or `acked`; `asc` or `desc` (default: the field's own direction) |
| `page`, `pageSize` | page from 1 (default 1); size `25`, `50` (default) or `100` |
| `like` | one incident ID: only the other incidents with the same error (see "Similar issues"); an ID kwatch no longer holds matches nothing |
| `node` | node names, repeatable: incidents about that node, or whose pods run on it (from the live inventory; without it nothing matches) |
| `mode` | failure families, repeatable: the first part of the incident's failure mode (`ImagePull` matches `ImagePull.Registry`); letters and digits only |

Without a severity filter, quiet (`silent`) active issues are left out.
Without a sort the order is kwatch's (see "Sorting"); every order ends
with the issue ID, so two reads of an unchanged list give the same
pages.

The response is `{incidents, total, page, pageSize, totalPages,
facets, modeLabels}`. `incidents` is the page; a page past the last is empty, with
the real `total`. `modeLabels` maps each requested `mode` to its plain label
(`{"CPUHigh": "CPU high"}`), which the page uses for the "Failure: …"
chip. `facets` has `severity`, `state`, `namespace`, `kind`,
`acked` and `stopped` (`true` / `false`), each a map of value to how many issues
of the filtered list have it, counted without its own filter (`state`
also without `status`, so it gives the size of each list; `severity`
leaves out the active quiet issues the list hides without a severity
filter, so it adds up to `total`); `namespace`
and `kind` hold the 50 and 20 busiest. Each item carries `rev`, a hash
of what its row shows. A list item never passes 590 bytes: its title
and cause are cut to 200 characters (the issue page has them whole),
then its free text is halved until it fits, so a full page of 100
always fits the 64 KiB response limit. The response carries an `ETag`
of its body; `If-None-Match` with it answers `304`. A value the API
doesn't know answers `400` (see "Errors").

### Errors

Every `400` from the dashboard API has the same body: `{"error", "param",
"term"}`. `error` is a plain sentence the page shows as it is, `param` the
query key at fault and `term` the offending value, cut to 64 characters
(the key itself when the whole key is wrong, such as `q` for a search
that is too long). Examples: "Too many search words: 10 is the limit."
(`q`, with the 11th word), "The search is too long: 200 characters is
the limit.", "Unknown failure type: Bogus." (`mode`), "Not a length of
time: \"soon\". Use a number and a unit, like 24h.", "Unknown state:
\"x\". Use settling, open, recovering, flapping, resolved.", "Page size
must be 25, 50 or 100, not \"x\".". The Issues page shows the sentence with
the term marked and a "Clear filters" button. The page itself drops
values it knows are wrong (see "The address"), so these show for links
written by hand.

### Similar issues

"The same error" is the incident's failure mode (`Mode`, the stable
identity of its root finding, such as `ImagePull` or `CrashLoop`) and,
when kwatch found a likely cause, the rule of that cause
(`Cause.Rule`): both must be equal (similar.go). kwatch stores this key
on the incident when it is announced (`incident.SameErrorKey`), so a
cause found or revised later does not move it to another group; an
incident not announced yet, or saved by an older kwatch, uses the key
of its mode and cause now. The mode is used
rather than the members' reason codes because kwatch keeps it across
resolves and restarts, while members leave as they recover. An incident
with neither has no key and is similar to nothing. The namespace and
object don't matter: the point is the same failure elsewhere.

`/api/incidents/{id}` carries `similar`, how many other incidents share
the key, counted as `/api/incidents?status=all&like=<id>` lists them
(active quiet issues left out, as on every list without a severity
filter), from at most 20,000 incidents read. The issue page and drawer
link "14 other issues have the same error → see them" to that list.

## Releases in the API

`GET /api/incidents/{id}` carries `release` when a release is linked to
the issue: the release's `id`, `kind`, `namespace`, `name`, `at`,
`images` (`container`, `before`, `after`), `changed`, `actor`, `app`,
`cause`, `verdict`, `final`, `rolledOut`, `rolloutSeconds`, `stalled`,
`deadline`, `ready`, `desired`, `oldServing`, `incident`, `newest` (no
later release of the workload), `undo` (`text`, `command`, `mutating`)
and, in Pro, `detail` (`restartsBefore`, `restartsAfter`,
`readyBefore`, `errorLine`). `GET /api/overview` carries `releases`:
`{total, bad, badIncidents, wrong}` for the last 24 hours (`bad` counts
regressed, failed and down; `wrong` is up to five of them with `id`,
`kind`, `namespace`, `name`, `at`, `verdict` and `incident`).
`GET /api/incidents?release=bad` lists the issues caused by a release
that went wrong in the last 24 hours.

## Map API

`GET /api/map?view=namespace|failing&namespace=&nodes=1`, read-only, with
the session and an `ETag` like the other reads; `400` for another `view`
or `nodes` value, `503` before the pipeline runs.

| Field | Data |
|:--|:--|
| `view`, `namespace` | the view drawn and, for `namespace`, the namespace (the one asked for, else the one with the most failing objects, else the first) |
| `namespaces` | the watched namespaces, for the picker |
| `nodes` | `{id, kind, name, namespace, level, ready, issue, external}`: `level` is `page`, `notify`, `digest` or `ok` (the worst open issue touching the object, quiet and resolved ones left out), `issue` its ID, `ready` "ready/wanted" for workloads, `external` an object of another namespace this one calls |
| `edges` | `{from, to, type}`: `routes`, `selects`, `calls`, `runs-on` (with `nodes=1`) |
| `tooMany`, `count` | more than 150 objects: nothing is drawn and `count` says how many |

## Activity API

The Activity page (`js/activity.js`, see "Activity", from
`GET /api/activity?since=1h|24h&untold=1`, default `1h`; Last hour /
Last 24 hours) lists, newest first, at most 20 changes, each a row like
"12:31 · New · Urgent · svc-204 cannot pull its image · team-6" linking
to the issue page. With none it says "Nothing changed in the last
hour." The server derives the events from the incidents it holds, with
its own clock; quiet issues are left out:

| Kind | Shown as | From |
|:--|:--|:--|
| `new` | New | the first open time kwatch kept (`Occurrences`, else `Opened`) |
| `reopened` | Came back | every later open time, a recurrence of an earlier incident included |
| `urgent` | More urgent | every tier rise the incident recorded (`transitions` of kind `tier raised`), and, before its first transition (an incident saved by an older kwatch), the timeline note "still failing: paging again" |
| `resolved` | Resolved | the resolve time, and those of earlier occurrences (`History`); a superseded incident is not "resolved" |

A recurrence carries the opens and resolves of the incident it repeats;
the same moment of the same root is listed once. Other changes of alert
level are not timestamped by kwatch, so they don't appear. The response
is `{since, events, total, untold, withUntold}` (each event: `at`, `kind`,
`id`, `title`, `severity`, `state`, `namespace`, `workload`,
`workloadKind`, the root object's kind as the list shows it, left out
when unknown, and `untold`, set on a change no message reached; the
fields are the incident as it is now), `total` counting every event
listed in the window. An event is untold when no message reached it: an
opening the incident's messages never covered (an earlier occurrence
not `Heard`, or an incident never announced and never told), a tier
rise or resolve of an untold incident, the resolve of an earlier
occurrence not heard. Untold events are left out unless `untold=1`;
`untold` counts them either way and `withUntold` says whether they are
in `events`. `untold` takes `1` or `0`; another value or a repeated
one answers `400` ("Unknown untold value: \"yes\". Use 1 or 0.");
ties are ordered by ID, so an unchanged set gives the same body, which
carries an `ETag` like the list. Another `since` answers `400` ("Unknown activity window: \"2h\". Use 1h or
24h.", see "Errors").

## Trends API

The Overview's "This week" (`js/trends.js`, from `GET
/api/trends?tzName=&tz=`, read with its `ETag`) takes from it the week
before's count, each namespace's openings, Kept coming back and the
median time to resolve; its week chart comes from `/api/insights`
`days` (see "Insights API"). The server computes
them from the incidents the dashboard already holds (active ones and
those resolved within the last week), with its own clock; quiet
(`silent`) issues are left out, as on the active list.

| Card | Data |
|:--|:--|
| Issues opened per day | each incident's kept open times (`Occurrences`: first open, reopens and recurrences, at most 30 per incident), bucketed into the 7 calendar days ending today; colored by the issue's alert level now. An opening no message reached (the incident settled, recovered or moved before alerting, or an earlier occurrence that was not heard) is counted as `untold` (in `total`, in no level), never as a page or a notification |
| Busiest namespaces | open times per namespace in those days, top 5; each links to `/issues?status=active&namespace=` when the namespace has an active issue, else `/issues?status=resolved&namespace=` |
| Kept coming back | issues opened twice or more in those days, or flapping now; top 5 by openings, then flapping, then repeat count; each links to its issue and names its namespace and object (`workloadKind` as on list items) |
| Time to resolve | median and longest time from open to resolve of every occurrence (the incident's `History` and the incident itself) resolved in those days; the longest links to its issue |

`tzName` is the dashboard's zone as an IANA name (`Africa/Cairo`: letters,
digits and `/ _ - +`, at most 64 characters; the browser's zone, or `UTC`
while the page shows UTC), so days start at midnight where the reader is
and a daylight-saving change within the week is counted right. When
`tzName` is missing, `Local` or a name kwatch doesn't know, `tz` is used:
the zone in minutes east of UTC (a whole number from -840 to 840), else
UTC. The page sends both. A name that can't be a zone name answers `400`. These cards are left out until at least 2
issues opened or resolved in the week (`enough`), and a card with nothing to show
is left out. They refresh with the Overview (on each live change, else every 20 s).
The response carries an `ETag` of its body; `If-None-Match` with it
answers `304`.

Limits: kwatch keeps a resolved incident for a week
(`incident.DefaultRemember`), and a recurrence replaces the incident it
repeats, so the trends never reach further back than that. For the same
reason the week before cannot be counted completely, and the summary
says only how many opened ("17 issues opened in the last 7 days"); the
comparison with the week before ("4 more than the week before") appears
only when incidents are kept for 14 days or more and kwatch holds data
from before that week. A count is how many times issues opened, not how
many different issues.

## Watch coverage

`GET /api/coverage` answers what kwatch watches, from the sources it
already runs (no API call per request), `503` while the pipeline is not
running; the body carries an `ETag` like the list. On the kwatch page,
"What kwatch is watching" (`js/ovcoverage.js`; the totals from
`/api/kwatch` `coverage`, and folded under them, read with its `ETag`,
"What it can't read, and the permission to add") says when kwatch finished reading the
cluster, the namespaces watched and left out (20 names shown, then
"+N more"), how many kinds it watches and has fully read, and the kinds
it can't read, hasn't read yet or doesn't watch, each with a plain
reason ("kwatch isn't allowed to read them (its role lacks
permission)", "not served by this cluster"). The body:

| Field | Data |
|:--|:--|
| `namespaces` | the namespaces the inventory holds, split by the namespace scope (`namespaces`, excluded namespaces and the namespace selector): `watched` and `excluded` counts, `watchedNames` and `excludedNames` (sorted, at most 50 each, `truncated` when cut), and `all` when nothing narrows the scope |
| `kinds.watched`, `kinds.synced`, `kinds.byMode` | running watches (typed informers and discovered kinds), how many hold their whole list, and the count per watch mode (`full`, `hashed`, `status`, `metadata`) |
| `kinds.typed` | every typed watch: `kind` (`pods`, `deployments.apps`), `mode`, `required`, `synced` (false too when the API refused it after it synced) |
| `kinds.unreadable` | kinds the API refused (`permission_denied`): kwatch's service account lacks the RBAC grant |
| `kinds.unsynced` | kinds whose first list is pending, timed out or failed (`cache_sync_pending`, `cache_sync_timeout`, `cache_sync_failed`) |
| `kinds.notWatched` | kinds turned off by configuration (`disabled_by_config`), not served (`api_unavailable`), over the watch budget (`watch_budget_exceeded`) or over their object cap (`object_cap_reached`) |
| `kinds.reasons` | the count of every gap by reason, including those the lists leave out |
| `syncedAt` | when every kind first finished its initial list (or gave up on it); absent while kwatch is still listing |
| `discoveryComplete` | false when the last discovery of the API's kinds was partial |

Each gap is `{kind, reason, detail, required, rule}`, `detail` a short
sentence; each list holds at most 50 entries (`kinds.truncated` when
cut; the discovered kinds are already capped by the dynamic source).
`kindTotals` (`unreadable`, `unsynced`, `notWatched`) is the full count
of each list, which the page uses for "+N more". For a
`permission_denied` gap, `rule` is the ClusterRole rule that would allow
it (`apiGroups`, `resources`, `verbs`), and `kinds.missingRules` holds
one such rule per API group. The page turns them into a "Permission to
add" box with the YAML to copy ("Add these rules to kwatch's ClusterRole,
or use rbac.mode=full in the Helm chart"). It is the same state `/health`
reports as `coverage`, with names; the reason codes are the same in both.

## kwatch itself

`GET /api/status` also says how kwatch itself is doing. `self` (left out
while no pipeline session runs) has:

| Field | Data |
|:--|:--|
| `startedAt` | when this session started |
| `lastStop`, `restarts` | how the previous session ended (`at`, `reason`, and `stoppedAt` when known), and the restarts of the last 24 hours, newest first. Reasons: `graceful_shutdown`, `leader_handoff` (lost the API server or the state lock), `internal_failure`, `oom_killed`, `eviction`, `node_disruption`, `deployment_rollout`, `api_unavailable`, `unknown` |
| `lastApiContact` | the last time kwatch reached the API server (needs leader election) |
| `stateFileBytes`, `outboxPending` | the state file's size and the messages accepted but not sent yet |
| `dashboardLinks` | `on`, and the `address` the alerts link to, or the `reason` links are off (`no_route`, `port_mismatch`, `unusable_address`, `routes_forbidden`) with `text`, a plain sentence |
| `podLogsForbidden` | kwatch may not read pod logs, so crash output isn't shown |
| `startup` | progress of the first read while `done` is false: `kinds`, `synced`, `objects`, `dynamicWatched`, `dynamicSynced` ... |

Beside it, `failedLastHour` (left out without alert delivery) counts the
provider sends that failed or are being retried in the last hour, counted
by the minute: incident messages, summaries, roll-ups, digests and plain
messages alike, however many one incident had. On the kwatch page, the
"kwatch itself" card shows running since, last restart and why ("normal
restart", "ran out of memory", "its node went away" ...), last API
contact, messages waiting and the restarts of the last 24 hours on a
time line; failed sends show as a notice. The Overview and the kwatch
page show "kwatch is starting" with "19 of 27 kinds read · 1,180
objects" until the first read is done, and "Alert links are off" with
`dashboardLinks.text` when alerts carry no link to this page.

`GET /api/kwatch` is kwatch's own status page in one read, with an
`ETag`. Each part is absent while its source is:

| Field | Data |
|:--|:--|
| `coverage` | totals of `/api/coverage`: `watched` (running watches), `synced`, `unreadable`, `unsynced` (first list not finished; these are among `watched`), `notWatched`, `total` (`watched` + `unreadable` + `notWatched`), `allNamespaces`, `namespaces`, `excludedNamespaces`, `syncedAt`; absent while the pipeline is not running |
| `delivery` | per alert channel, by name: `{name, provider, kind, sent, failed, retrying, lastSuccess, lastError}`. The counts are the incident messages of the last `deliveryWindowHours` (24) from the send trail kwatch keeps per incident (the 512 incidents told last, 16 attempts each, so a busy day can count less); `failed` were given up or handed to the fallback. `lastSuccess` and `lastError` are the channel's own health, summaries and digests included. Every channel the delivery health knows is listed, even without sends |
| `stateFile` | `bytes` (the state file's size), `capBytes` (the size cap the compactor keeps the stored data under: 512 MiB by default) and `capKnown`; while no session runs it is absent |
| `likelyMisses` | the cause self-check's likely wrong causes of the last 7 days, newest first, at most 50 (Pro; absent otherwise): `{id, rule, root, other, change, opened, resolved, exportSince, exportUntil}`, `other` and `change` saying what changed instead, the export window 10 minutes either side of the incident |
| `resources` | kwatch's own container, read from its cgroup (v2, else v1) every 15 s, so it needs no metrics-server and no extra permission: `memory` `{used, limit}`, `cpu` `{cores, limitCores}` (use between the last two samples; absent until there are two), `disk` `{used, total}` of the volume that holds the state file, and `stateFile` as above. `limit` and `limitCores` are absent when the container has none; a part kwatch can't read is absent, and `resources` is absent when nothing is known. `disk` and `stateFile` need a state file |
| `restarts24h`, `outboxPending` | the restarts of the last 24 hours and the messages waiting, as in `self` |
| `failedSendsLastHour` | as `failedLastHour` |

## Overview API

`GET /api/overview` is the top of the Overview in one read, with an
`ETag` (`304` on `If-None-Match`), `503` while the pipeline is not
running. It reads the incidents the dashboard holds (at most 20,000).

| Field | Data |
|:--|:--|
| `counts` | the issues not resolved by severity: `urgent` (page), `attention` (notify), `low` (digest), `quiet` (silent) |
| `lookFirst` | at most 5 active issues (not resolved, not quiet): the urgent ones nobody acknowledged first, then the newest started, then by ID; each `{id, title, namespace, kind, name, severity, state, since, acked}` (`kind`/`name` are the root object's, `title` at most 200 characters) |
| `failedSendsLastHour` | the provider sends that failed or are being retried in the last hour, as `failedLastHour` on `/api/status`; absent without delivery |

## Insights API

The Overview's "This week" comes from `GET /api/insights?tzName=&tz=`
(both as on `/api/trends`; `js/ovinsights.js`, see "Overview"). A card
with nothing to show is left out. The server computes everything from the incidents
it holds (at most 20,000 read) with its own clock; "active" means not
resolved and not quiet.

| Field | Card | Data |
|:--|:--|:--|
| `today` | the "Today:" line | `new` (openings today, reopens included), `resolved` (resolves today, earlier occurrences included, a superseded incident not), `urgent` (urgent issues active now); quiet issues left out; hidden when all three are 0 |
| `unacked` | not drawn now (the Overview's "Look at these first" uses `/api/overview`) | `count` and the 5 newest urgent active issues with no acknowledgement; "See all" opens `/issues?status=active&severity=page&acked=false&sort=started` and "Oldest first" the same with `dir=asc` |
| `afterChange` | Started right after a change | active issues with a change they stored (`Journal.Changes`) within 30 minutes before they started: `count`, `total` (every active issue, for "3 of 8"), and the 5 newest with the changed object and what changed ("After: deployment/api image api:1.2 → api:1.3 (4 min before)"); values redacted, a data key named only |
| `undelivered` | not drawn now (failed sends show on the kwatch page) | from the persisted send trail of every incident held: per incident, provider and message kind, the latest attempt of the last 24 hours when it `failed` (gave up) or is `retrying`; `count` and the 5 newest with provider, issue and the scrubbed error (redacted, 200 characters) |
| `nodes` | Nodes with the most issues | the 5 nodes with the most active issues (the issue is about the node, or its pods run there, from the live inventory), with how many are urgent; each links to `/issues?status=active&node=` |
| `causes` | Top causes this week | the 5 failure families with the most issues kwatch holds (it keeps a week), counted as `/issues?status=all&mode=` lists them (active quiet issues left out), with a plain label ("Image pull failures", "Out of memory"; other families split into words) and how many are still active |
| `namespaces` | Namespaces with active issues | one tile per namespace with active issues: its most urgent severity and count, with `urgent`, `attention` and `low` counts, most urgent first, then most issues, then A to Z; at most 60 (`more` counts the rest). Each tile is a link to `/issues?status=active&namespace=`, in a list, with the count and severity in words, not color alone. Cluster-wide issues have no tile |
| `days` | the week chart | the 7 calendar days ending today in the requested zone (`tzName`, else `tz`, as on `/api/trends`, so a daylight-saving change is counted right): `{date, opened, resolved, paged, untold}`; `opened` counts every kept opening (reopens included), `resolved` every resolve (earlier occurrences included, a superseded incident not), `paged` the openings that paged (an earlier occurrence that paged, or the current one while the issue is urgent and a message of it went out), `untold` the openings no message reached (settled, recovered or moved before alerting, or an earlier occurrence that was not heard; counted as `untold` on `/api/trends`, and also in `opened`, always present, 0 when none); quiet issues left out |
| `quiet` | not drawn now | only when nothing is active: `objects` (the inventory's size), `namespaces` and `kinds` watched (coverage) and `lastScan`, with the scope from `/api/status` in the Issues page's words: "Watching every namespace (34 namespaces now) · 2,841 objects · last checked 10 s ago · What kwatch is watching" |

The body is bounded (5 entries per list, 60 tiles, texts cut to 200
characters) and carries an `ETag`; it refreshes with the Overview.

## Live push

`GET /api/events` is a Server-Sent Events stream behind the same
sign-in as the rest of `/api/*` (`text/event-stream`,
`Cache-Control: no-store`, `X-Accel-Buffering: no` so proxies pass it
through; every event is flushed). It sends:

- `retry: 5000`, then `ready` with `{"version"}`, a fingerprint of
  every incident (its rows fingerprinted redacted, as the API shows
  them, so the version says nothing about a redacted secret);
- `changed` with `{"version", "ids", "more"}` whenever incidents
  change: the IDs added, removed or changed since the stream's last
  event (what a row shows, the messages sent, the timeline length and
  the members), at most 50, sorted; `more` when there were more, or the
  stream fell behind the last 32 changes, so the page reads everything
  again;
- `ping` every 25 s;
- `signedout` (empty data) right before the stream closes when its
  sign-in is no longer valid.

How it learns of changes (`live_hub.go`): while a stream is open, each
stream's 2 s ticker asks the hub to compare the incidents, which it does
at most once per 2 s and only after the engine ran again (`NoteScan`,
called on every loop of the engine); the hub has no goroutine of its
own and forgets everything when the last stream closes. At most 50
streams are open at once (the 51st gets `503` with `Retry-After: 30`),
and at most 3 from one client, a valid session or, for the bearer token
(or a cookie that is not a valid session), an address (the 4th gets `429` with `Retry-After: 30`);
a stream ends when the client leaves, when the server shuts down (the
shutdown closes them, so it never waits for them), when the sign-in is
no longer valid (checked every 2 s, after a last `signedout` event) and after 72 heartbeats
(30 minutes), after which the browser reconnects. Each write has its
own 10 s deadline, so the server's write timeout doesn't cut the
stream. The Content Security Policy needs no change: `connect-src
'self'` allows the same-origin `EventSource`.

The page (`js/refresh.js`) opens one stream per tab, closes it while
the tab is hidden and opens it again when it is shown. On `changed`,
every view's reader runs (at most once per 5 s; changes that come
faster are read together): the table page, the Overview and
the open issue, each with its `ETag` where it has one. A reader may
pass `wants(ids)` to `KW.poller` to run only for the incidents it
shows. While the stream is connected the readers run on their own only
every 60 s, as a safety net. Fallback: without `EventSource`, or when
the stream is refused (`401`, `503`, a proxy that blocks it), fails,
or says nothing for 60 s (a proxy that buffers it), the readers go back
to their own interval (20 s); a closed stream is tried again after 30 s,
then longer, up to 5 minutes; showing the tab again does not cut that
wait short. When the stream (re)connects, a reader that ran in the
last 5 s does not run again, and a reader never starts a second read
while one is out (it runs once more after it). On `signedout` the page
shows the sign-in at once and does not reconnect. Every stream message counts as a successful read for the
offline banner and the freshness line, so "updated just now" stays
true while the stream is quiet.

## Access and limits

- **Failure limit.** A wrong password or bearer token counts against
  the client: 10 a minute, then `429` with `Retry-After` until the
  minute ends, even for a right one, so a lucky guess cannot get in.
  Each sign-in and each bearer request reserves its attempt before it
  is checked (a right one gives it back), so many slow requests sent at
  once cannot all slip past the limit. A valid session cookie always
  works, even beside a wrong bearer token, and a stale one never
  counts. Only an `Authorization: Bearer` header (the scheme in any
  case) is a bearer token; any other scheme is ignored. The limiter
  remembers at most 10,000 clients a minute; more share one bucket.
- **Asking whether you are signed in.** `GET /api/session` is the one
  `/api/*` route that does not answer `401` to a visitor with no
  credentials: the sign-in page asks it on every load, and a `401`
  would show as a console error. With no bearer token and no valid
  session cookie (none, expired, signed out or made up) it answers
  `200` with `{"signedIn": false}` and nothing else, and it does not
  count against the failure limit. A valid session answers `204`. A
  request that carries an `Authorization: Bearer` header goes through
  the usual check, so a wrong token still gets `401` and counts. Every
  other `/api/*` route still answers `401` when signed out. The answer
  is `Cache-Control: no-store` like every other.
- **Behind a trusted proxy** (`dashboard.trustProxy`), the client is
  the address the proxy appended to `X-Forwarded-For` (a value that is
  not an IP address is ignored and the peer is limited instead), and
  only that client is limited: guesses from some clients never lock out the
  others. Failures through one proxy are counted only to log a warning
  ("many dashboard sign-in failures through one proxy") when 100 come
  in a minute. Without `trustProxy` behind an Ingress every client
  shares the proxy's address and its one limit.
- **Shared mode** (`dashboard.mode: shared`) refuses sign-in over plain http
  (`403 https_required`) and refuses the bearer token over plain http
  too (`403`, "the token must be sent over https"), except from
  loopback, which is how a `kubectl port-forward` arrives. Over https
  every response carries `Strict-Transport-Security: max-age=31536000`
  (subdomains are left alone).
- **Alert links** use the scheme signed-in requests reached each host
  with (`scheme.go`): requests that are not signed in record nothing,
  so made-up `Host` headers cannot fill or flush the 16 hosts it
  remembers, and once a host was reached over https a later plain
  request does not turn it back to http.
- **Unknown addresses.** A path outside `/api/` and `/assets/` that the
  UI does not have answers `404` with a small page in the dashboard's
  own look, titled "Not found - kwatch" (the shared stylesheet and
  brand by their hashed asset links, no script or inline style), linking
  to the Overview; `/api/*` and missing assets keep the JSON error. A path
  that can't be parsed at all (a broken escape such as `%E0%A4%A`) gets
  the same page, still with status `400`, on plain http and when kwatch
  serves TLS itself (`dashboard.tlsCertFile`).
- **Access log.** A successful read is logged at verbosity 2, a `401` or
  `404` at verbosity 1 (anyone can cause them at will), and sign-in,
  sign-out and the other failures at the default level. Headers are never
  logged.
- **Redaction cache.** List reads redact titles and causes through a
  cache of two generations of 8192 texts: when one fills it becomes the
  old one, and a text read from the old one moves to the new one, so a
  cluster with thousands of incidents does not redact everything again
  on each read, and the cache never holds more than 16384 texts.

## When kwatch can't be reached

Every API answer passes through `js/refresh.js`. A network failure, a
timeout or a `5xx` (or no successful read for 45 s while the tab is
shown) shows a bar at the top of every page, `aria-live="polite"`:
"Can't reach kwatch. Showing data from 12:03 (5 min ago). Retrying…"
with Try now, which reads everything on the page again. When the
browser is offline it says "You're offline. Showing data from 12:03."
The time follows the Local / UTC choice. The table, the Overview and
the issue (page or drawer) keep what they showed, dimmed a little. The
freshness line never says "Live" then: it shows an "Offline" pill and
"last updated 2 min ago" and keeps ticking (the bar's Try now is the only
one). On the Issues page, when you change a filter while kwatch can't be
reached, the rows stay those of the view before and a note says "Showing
the last list. This filter applies when kwatch is reachable." The
Overview's headline says "Can't reach kwatch — last known at 19:44"
instead of "All clear" or the counts' sentence, and its health line
says it can't tell whether alerts are getting out. The next answer
that works hides the bar.
`401` is not a failure: it goes to sign-in as before. A request the
page canceled itself (a newer search) counts for nothing.

## Look and brand

The brand tokens are `web/tokens.css`, a copy of
`kwatch.dev/packages/brand/tokens.css`; no other stylesheet redeclares
one (`look.css`'s high-contrast overrides aside). `tokens_test.go` fails
when the copy differs from the brand file (when the two repositories sit
side by side, or `KWATCH_BRAND_TOKENS` names it), when a stylesheet
redeclares a brand token, uses an undeclared `var(--x)`, holds an
`rgb()` color or a blurred drop shadow. `app.css` adds only what is the
dashboard's own: `--max-table`, `--ctl` (one 36px control height, 44px
on a touch screen), `--text-scale` and the `--t-*` sizes (the brand's
sizes times the View options text size), and `--ink-failing` /
`--ink-degraded`, the dark-mode red and amber for log lines on the
always-dark `--ink` surface (the brand has no token for that).

Signal (lime) is on the logo dot only. The active nav bar and an
updated row use `--text`; a search match is `--surface` with an
underline. No shadows: menus, the drawer and bars have a 1px `--line`
border. `--radius-pill` is for buttons; chips, counts, the switches and
filter chips use `--radius-chip`. Inputs and pickers have one border,
`--control-border`. A hover on a row or card is `--surface`; a text link
grows its underline from 1px to 2px with no color change. Information is
visible text, never only a tooltip. A tab title reads "Active issues -
kwatch".
