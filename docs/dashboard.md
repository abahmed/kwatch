# Dashboard UI notes

Implementation notes for the embedded dashboard UI in
`internal/dashboard/web` (plain HTML, CSS and JavaScript, no build step,
strict CSP, nothing loaded from elsewhere). The published user guide stays
on [kwatch.dev](https://kwatch.dev/docs).

## Reading order

The UI is written for someone new to kwatch: the first thing on each page
says what is wrong in one sentence, and the rest waits behind a click.

- **Navigation**: the sidebar has **Overview** (`/`, the landing page)
  and **Issues** (`/issues`, with the count of active issues). Old
  addresses keep working: the server (and the page, for Back to an old
  history entry) sends `/status` to `/`, `/resolved?…` to
  `/issues?status=resolved&…`, and `/` with list filters (`q`,
  `severity`, `namespace`, `state`, `age`, `since`, `sort`, `dir`,
  `group`, `kind`, `acked`, `status`, `offset`, `limit`, `page`,
  `pageSize`) to `/issues?…`
  with the
  same query, so bookmarks and links pasted in chat open the same list.
  An issue stays at `/issues/<id>`.
- **Issues**: one table for active and resolved issues (see "Issues
  table" below), showing the active ones first.
- **Issue page**: a "What's happening" card (severity, state, title,
  "Likely cause: …" right under the title (from `rootCause`), where,
  since when, up to five hint lines, and "What to do first" with the
  first fix step and its copyable command). The hint lines are
  "Changed 4 min before: …" (the change the cause names: the one the
  API marks `cause`, else the one the checks' "Change" names, else
  the nearest before it started), "Seen 3 times before in 30 days", who
  was told ("PagerDuty failed (401) · fell back to Opsgenie 19:06 ·
  Slack 18:51 · not acknowledged": failures first, each channel with
  the time it first took a message), "Also causing: 2 issues" and "3
  other issues started within 5 minutes of this one"; each opens its
  section. Everything else is in closed sections: "Why kwatch thinks
  this" (likely cause and, with Pro, the evidence), "Other issues"
  ("Also causing", then "Started at the same time", leaving out issues
  already named as caused by or causing this one), "All steps to fix",
  "What changed just before", "Seen before", "Why it's waiting to
  start", "Failing probe", "Recent warnings", "Traffic impact", "Node",
  "What happened, step by step" (a states strip, then the list; events
  in the same minute share one line), "Logs" (one view with one "Show"
  picker: the lines kept when it started and, with Pro, each container
  run a check read, the matched line marked), "Containers" (the table,
  then memory and CPU now, then the memory or CPU of the last 24 hours
  when it is evidence), "Affected objects", "Who was told" and "More
  details" (alert level, start time, acknowledgement, ID). A section
  with no data is left out. The section that is the explanation opens
  by itself on the first draw: "Failing probe" for a probe failure,
  "Why it's waiting to start" for a pod that can't be placed, and
  "Containers" when the memory chart is evidence; it does not stop the
  issue from redrawing until the reader clicks it. Whether someone
  acknowledged it is said in "What's going on with this alert", with
  the command that does it. Times are hh:mm; the seconds are in the
  tooltips. The old `?tab=evidence` link opens "Why kwatch thinks
  this". Under the facts, "14 other issues have the same error → see
  them" (see "Similar issues") when there are any.
- **Overview** (`/`), an incident view: what is broken now, why,
  what to do and who was told. One headline ("1 urgent problem needs
  attention." or "All clear. Nothing is failing."; while kwatch can't
  be reached "Can't reach kwatch — last known at 19:44" instead, see
  "When kwatch can't be reached"), today in one line ("Today: 18 new,
  22 resolved, 3 still urgent."), or on a quiet cluster what kwatch
  watches (in the same words as the Issues page's empty state:
  "Watching every namespace (14 namespaces now) · 2,841 objects") and
  when it last looked, counts by severity that link to the Issues
  table filtered to them (`/issues?status=active&severity=page`), the
  insight cards with the one urgent list, "Urgent and not
  acknowledged" (newest first, with "See all" and "Oldest first"), and
  the namespace map (see "Overview insights"), "What changed recently"
  (see below), "This week", folded (see "Overview trends"), one line
  on whether kwatch itself works and alerts get out (it also says when
  kwatch isn't allowed to read some kinds: "kwatch isn't allowed to
  read secrets and configmaps, so it can't see problems there"),
  "kwatch health details" (version, alert channels), which opens by
  itself when a channel fails or a check is late, and "What kwatch is
  watching" (see "Watch coverage"), which opens by itself, once, when
  kwatch isn't allowed to read some kinds.
- The **?** beside the logo explains the colors and alert levels, the
  states (Happening now, Confirming, Keeps coming back, Looks better,
  Resolved, in `KW.STATE`'s words) and lists the keyboard keys (press
  `?` to open it). It opens above the page, the table heading
  included, and scrolls inside when the window is short.
- **Phones** (under 700 px wide): the sidebar's notifications, times
  and sign out fold behind one "Settings" button, and the Issues
  filters (quick views, search, pickers, severity chips) behind one
  "Filters (2)" button, so rows show on the first screen.
- **Formats** (`js/core.js`): one number format (`KW.num`: "4,812"),
  one object format (`KW.obj`: "deployment/payments-api", the
  namespace shown on its own, "whole cluster" without one) and one
  clock format (`KW.hm`: "14:02", "yesterday 14:02" or "12 Oct,
  14:02", the exact time with seconds in the tooltip) on every page
  and in notifications.

Visible words replace internal terms; the JSON field names do not change:

| API value | Shown as | Tooltip |
|:--|:--|:--|
| `severity: page` | Urgent | Alert level: alerted the on-call person |
| `severity: notify` | Needs attention | Alert level: sent a message |
| `severity: digest` | Low | Alert level: in the daily summary |
| `severity: silent` | Quiet | Alert level: not sent |
| `rootCause` | Likely cause | kwatch's best guess |
| `ackedBy`, `acked`, `ackedSince` | Acknowledged | acknowledged with the annotation: the object that carries it, and since when kwatch saw it (`ackedSince` is absent for incidents saved by an older kwatch) |
| correlation | What the checks agree on | |

## Issue page context

`/api/incidents/{id}` carries these parts besides the incident itself.
Each is omitted when there is nothing to show, bounded where it is built,
cut further by the response size limit (64 KiB) in a fixed order, and
counted in `truncated` when entries are left out.

| Field | Section | Source | Bound |
|:--|:--|:--|:--|
| `changes`, `fixedBy`, `fix` | What changed just before | the changes the incident stored while it was live (first), merged with the inventory's change history around the root (what it uses, routes to or scales it, its node), from 30 minutes before the failure began until it resolved; `fixedBy` is the change kwatch judged fixed it, and a change with `cause` is the one the likely cause blames (the page marks it "Likely cause" and names it in the summary card). Each change carries `fields` (`path`, `before`, `after`) for a diff: at most 6, values redacted and cut to 160 bytes, and the data of a Secret or ConfigMap names its key only (`after` is `added`, `changed` or `removed`) | 8, before first, then during |
| `seenBefore` | Seen before | the incident's own history of earlier occurrences of the same root (kwatch keeps the last 30) | 30 |
| `snapshot`, `snapshots` | When it broke | the failing containers (not ready, restarted, or waiting or ended for a reason) of the incident's pods, stored when it opened (`reason: "first break"`) and replaced each time it got worse (`"got worse"`: its tier rose or it reopened); same row fields as `containers`, as they were then. `snapshot` is the latest of `snapshots` (oldest first) | 2 snapshots, 10 containers each |
| `transitions` | the states strip | every state and tier change: `at`, `kind` (`opened`, `announced`, `recovering`, `failing again`, `flapping`, `reopened`, `resolved`, `state changed`, `tier raised`, `tier lowered`), `from`, `to` (state names, or tier names for a tier change) | 50 newest |
| `why` | Why this level, why still open | `alertLevelReason`: why the incident is at its alert level, in plain words (a page rule, a known or routine failure, a configured override, a level kept from earlier …); `stillOpenReason`, e.g. "Looks better; closes after 3m healthy, 1m20s so far", "Confirming: making sure it still fails before alerting, 40s left", "Flapping: waiting for 30m stable, 5m so far"; `holdTotal` and `holdElapsed` in seconds; `nextCheckAt`, when kwatch next looks at it on its own. Timers come from kwatch's latest tick; a resolved incident has only `alertLevelReason` | |
| `ack` | Acknowledged | `note` (the annotation's text, redacted), `since` (when kwatch saw it or its note change), `by` (the tool that wrote it: the field manager the object's managed fields name for the `kwatch.dev/ack` key, such as `kubectl-annotate`; absent when they name none) and `on` (the object that carries it) | |
| `causedBy`, `causing` | Caused by, Other issues | the other open incidents this one is caused by and the ones it causes, as the reasoning ties them (a failure chain that goes on past its limit into the other's failures, or a cause rooted at the other's root): `id`, `title` (the headline of its last message, else its root), `severity`, `state` | 5 each |
| `blastRadius` | Who it hits | one line from the impact and the failing pods, such as "Affects 2 services, 1 ingress and 3 of 4 pods of payments"; once nothing fails (or after the resolve), the widest it reached | |
| `phases` | the states strip | that history and the incident's open, recovering and resolve times | the last 24 hours |
| `containers` | Containers | the live inventory: the containers of the root's and failing objects' pods, with requests, limits and kubelet usage | 10, most restarted first |
| `affected` | Affected objects | root (`root`, shown as "this issue's object"), the object the likely cause blames when it is not the root (the object of its blamed change, `status: cause`, shown as "Likely cause"), merged roots, failing objects (a container counts as its pod) and impact, with `kubectl` commands | 20 |
| `deliveries` | Who was told | delivery's send trail per incident, saved in the state file (provider, kind, placement, result, retries, scrubbed error, and `fallbackFor`, the provider a fallback stood in for; both named as products are, "PagerDuty"); rows read "Slack · update sent" | 16: the newest and the first accepted |
| `warnings` | Recent warnings | Kubernetes Warning events (inventory notes) on the root, merged roots, failing objects, impact, pods and their containers, from 30 minutes before it opened; folded by reason and redacted message, with count, last seen and the object | 10 newest |
| `traffic` | Traffic impact | the Services among the incident's objects or selecting / routed to its pods, and the Ingresses routing to them: ready vs total endpoints summed over their EndpointSlices now; a Service is left out when no EndpointSlice is known in its namespace | 8, not-ready first |
| `nodes` | Node | the root node, or the nodes the incident's pods run on: zone, pool, instance type, spot / on-demand (`karpenter.sh/capacity-type`, `eks.amazonaws.com/capacityType`, `cloud.google.com/gke-spot`, `cloud.google.com/gke-preemptible`, `kubernetes.azure.com/scalesetpriority`), age, unhealthy conditions (redacted), the incident's pods on it and other pods on it not ready | 3 nodes, 6 conditions, 256 pods read per node |
| `probes` | Failing probe | the containers of liveness / readiness / startup probe findings: check (`http /path :port`, `tcp :port`, `grpc :port`, `exec <program>`; exec arguments are never recorded), delay, timeout, period, failure threshold, and the latest `Unhealthy` event (redacted) | 4 |
| `timing` | "What's going on with this alert" | `startedAt` (the earliest start of its failure findings, as Kubernetes reported it, never after it opened), `noticedAt` (when kwatch opened it), `toldAt` and `toldVia` (the first message a channel accepted at or after the open, from the delivery trail) | |
| `sameTime` | Other issues ("Started at the same time") | the other incidents that opened within 5 minutes of this one (`id`, `title`, `severity`, `state`, `namespace`, `startedAt`, `relation`): `same node` (a node they share: a root, merged root or the node their pods run on; checked for the 64 nearest), then `same namespace`, then `cluster`, nearest start first; active quiet issues are left out | 8 |
| `usageHours` | Containers ("Memory, last 24 hours", or CPU) | only as evidence: memory when a finding is about memory (OOMKilled, memory high) or a container was killed for it or uses 90 % or more of its limit; CPU only for a CPU throttling finding. Per container: `metric` (`memory` in bytes, `cpu` in millicores), `limit`, and `peaks` (`start`, `value`), the kubelet stats poller's hourly peaks, oldest first | 3 histories, 24 peaks each |
| `waiting` | Why it's waiting to start | the incident's pods that are Pending with `PodScheduled=False`: the newest `FailedScheduling` event, else the condition message (redacted), parsed into plain-words reasons with node counts, plus the raw message | 1 message, 8 reasons |

Fields another part of kwatch may add are drawn when present and
ignored when absent: `causedBy` (one issue or a list, each an `id` and
`title`) as "Caused by: …" in the summary card, `causing` as "Also
causing: 3 issues" (a hint line and a section listing them), and
`blastRadius` (a sentence, or counts) as "Who it hits: …".

**What's going on with this alert** (`js/incwhy.js`) sits under the
summary card: why it is still open in plain words ("Looks better ·
closes after 3 min healthy · 1 min 20 s so far") with a bar that fills
each second between reads while `holdTotal` and `holdElapsed` run (a
settling issue counts its "40 s left" down); "Why you got this alert:"
with `alertLevelReason`; "Started 12:30 · kwatch noticed 12:30 ·
told Slack 12:31" from `timing` (each with a tooltip that has the
seconds); and the
acknowledgement, "Acknowledged 12:40 by kubectl-annotate · “note”", or,
when nobody has, the command that sets it
(`kubectl -n <ns> annotate <kind>/<name> kwatch.dev/ack="looking into
it"`) and the one that removes it (`kwatch.dev/ack-`): the dashboard
only reads. The timers are left out of the comparison that decides
whether the issue changed, so a ticking timer never shows "This issue
changed"; a new read moves only the timer.

The states strip (`js/transitions.js`) is drawn from `transitions` when
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
next visit the summary card says "3 things changed since you last
looked (12:05)" and the newer timeline entries, transitions, changes and
warnings carry a "New" mark. The first visit marks nothing; without
`localStorage` nothing is marked.

The summary card's **Copy summary** button copies, built in the browser
from this response: the title, severity in plain words, state when not
"happening now", where, since (UTC), likely cause, the first fix step and
its command, and a link to the page.

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
*All*, each with its count. Resolved and All show "Resolved within"
(last 24 hours, 3 days, or 7 days: kwatch keeps resolved issues a
week); it narrows resolved issues only, never active ones.

**Columns.** By default: severity badge, the issue's title (one line,
cut with "…"; the tooltip has the whole title and the likely cause;
after it, small marks for how often it failed again, `× 3`, and New /
Changed since your last visit), namespace ("whole cluster" when there
is none), object (`deployment/payments-api`), status, when (on Active
"Going on": "for 3 h", or "now" under a minute, the exact start in
the tooltip; on Resolved "Resolved": "3 h ago"; "Going on or
resolved" on All) and acknowledged. Start time and duration can be
added in the Columns panel. The Resolved list has no Status column
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
px wide the Object column hides and the sidebar and page margins
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
the presets *Compact* (severity, issue, namespace, status, when),
*Standard* (the default) and *Everything* set them in one click, and
*Reset columns* goes back to the standard set, order and widths.
Severity and Issue can't be hidden; severity stays first, since its
color strip marks the row. Each heading also has a menu (its ⋮ button,
shown on hover or focus, or a right-click on the heading): the two
sort directions, Hide column, Move left, Move right. It stays open
while the table updates. A
heading's right edge drags to resize the column (56 to 640 px; arrow
keys on the focused edge resize by 16 px; a double-click fits it to
what the page shows). Columns nobody resized fit their content (the
short ones) or take a share of the table (namespace, object); the
title takes the rest. When the chosen columns can't fit, the table
scrolls sideways inside its frame, never the page. The headings stay
on screen while the rows scroll. Columns, order and widths are kept in
this browser (`cols`, `order`, `widths` in `kwatch.view`) and in each
saved view. Below 700 px, where rows are cards, the same columns are
the lines of each card.

**Filters and counts.** The search box (with the shortcuts below), the
multi-select pickers **Namespace**, **Kind**, **State** and
**Acknowledged**, the **Started** picker (one choice: any time, today,
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
border and shadow, light and dark. The native `<select>` stays in the
page, hidden, and keeps the value, so code and the address read and
set it as before; a button with a listbox (arrows, Enter or Space,
Esc and Tab close and return focus) replaces its look. A select added
to the page later is picked up by itself.

**Quick views** are one-click presets above the table: *New today*,
*Ongoing over 1 day* and *Recently resolved* (resolved in the last 24
hours); the Urgent severity chip shows only urgent issues. They only
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
shows the same content as the issue page: `detail.js` has one renderer
(`KW.renderIncident`) and `incpanel.js` loads and refreshes an issue
for both, so the page and the drawer never differ. Its bar has
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
waits for "Show"), and the drawer's issue reads again every 30 s on its
own.

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
the next and previous issue on an issue page or in the open drawer),
`[` / `]` do the same on an issue page and in the drawer, Enter opens
the issue in the drawer and Esc closes it. `?` opens the help
beside the logo, which lists these keys; Esc closes it.

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
then opens. A link with a query always wins. **Reset to defaults** puts
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
| `q`, `exclude` | search terms, repeatable: every `q` must match, no `exclude` may (at most 10 terms and 200 characters in all; more terms answer `400` "too many search words") |
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
facets}`. `incidents` is the page; a page past the last is empty, with
the real `total`. `facets` has `severity`, `state`, `namespace`, `kind`
and `acked` (`true` / `false`), each a map of value to how many issues
of the filtered list have it, counted without its own filter (`state`
also without `status`, so it gives the size of each list); `namespace`
and `kind` hold the 50 and 20 busiest. Each item carries `rev`, a hash
of what its row shows. A list item never passes 590 bytes: its title
and cause are cut to 200 characters (the issue page has them whole),
then its free text is halved until it fits, so a full page of 100
always fits the 64 KiB response limit. The response carries an `ETag`
of its body; `If-None-Match` with it answers `304`. A value the API
doesn't know answers `400` with a plain message such as
`{"error": "invalid sort"}`.

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

## What changed recently

On the Overview, "What changed recently" (`js/activity.js`, from
`GET /api/activity?since=1h|24h`, default `1h`; Last hour / Last 24
hours) lists, newest first, at most 20 changes, each a line like
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
is `{since, events, total}` (each event: `at`, `kind`, `id`, `title`,
`severity`, `state`, `namespace`, `workload`; the fields are the
incident as it is now), `total` counting every event in the window;
ties are ordered by ID, so an unchanged set gives the same body, which
carries an `ETag` like the list. Another `since` answers `400`
`{"error": "invalid since"}`.

## Overview trends

"This week", a folded section of the Overview (`js/trends.js`, from
`GET /api/trends?tz=`, read with its `ETag`), shows four read-only
cards, and the top causes of the week (see "Overview insights"). The server computes
them from the incidents the dashboard already holds (active ones and
those resolved within the last week), with its own clock; quiet
(`silent`) issues are left out, as on the active list.

| Card | Data |
|:--|:--|
| Issues opened per day | each incident's kept open times (`Occurrences`: first open, reopens and recurrences, at most 30 per incident), bucketed into the 7 calendar days ending today; colored by the issue's alert level now |
| Busiest namespaces | open times per namespace in those days, top 5; each links to `/issues?status=active&namespace=` when the namespace has an active issue, else `/issues?status=resolved&namespace=` |
| Kept coming back | issues opened twice or more in those days, or flapping now; top 5 by openings, then flapping, then repeat count; each links to its issue and names its namespace and object (`workloadKind` as on list items) |
| Time to resolve | median and longest time from open to resolve of every occurrence (the incident's `History` and the incident itself) resolved in those days; the longest links to its issue |

`tz` is the dashboard's zone in minutes east of UTC (`0` when it shows
UTC, the browser's offset otherwise; a whole number from -840 to 840,
default 0), so days start at midnight where the reader is. The offset
is the one of today: a daylight-saving change within the week shifts
the older days by an hour. The section is hidden until at least 2
issues opened or resolved in the week, and a card with nothing to show
is left out. It refreshes with the Overview (on each live change, else every 20 s).
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
running; the body carries an `ETag` like the list. On the Overview,
"What kwatch is watching" (`js/ovcoverage.js`, folded; read with the
Overview and its `ETag`, since the health line says when kwatch isn't
allowed to read some kinds, and then it opens by itself once; the
quiet line links to it) says when kwatch finished reading the
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

Each gap is `{kind, reason, detail, required}`, `detail` a short
sentence; each list holds at most 50 entries (`kinds.truncated` when
cut; the discovered kinds are already capped by the dynamic source). It
is the same state `/health` reports as `coverage`, with names.

## Overview insights

Under the headline, short cards from `GET /api/insights?tz=` (`tz` as
on `/api/trends`; `js/ovinsights.js`). Each card is left out when it
has nothing to say. The server computes everything from the incidents
it holds (at most 20,000 read) with its own clock; "active" means not
resolved and not quiet.

| Field | Card | Data |
|:--|:--|:--|
| `today` | the line under the headline | `new` (openings today, reopens included), `resolved` (resolves today, earlier occurrences included, a superseded incident not), `urgent` (urgent issues active now); quiet issues left out; hidden when all three are 0 |
| `unacked` | Urgent and not acknowledged, the Overview's one urgent list | `count` and the 5 newest urgent active issues with no acknowledgement; "See all" opens `/issues?status=active&severity=page&acked=false&sort=started` and "Oldest first" the same with `dir=asc` |
| `afterChange` | Started right after a change | active issues with a change they stored (`Journal.Changes`) within 30 minutes before they started: `count`, and the 5 newest with the changed object and what changed ("After: deployment/api image api:1.2 → api:1.3 (4 min before)"); values redacted, a data key named only |
| `undelivered` | Alerts that didn't get out | from the persisted send trail of every incident held: per incident, provider and message kind, the latest attempt of the last 24 hours when it `failed` (gave up) or is `retrying`; `count` and the 5 newest with provider, issue and the scrubbed error (redacted, 200 characters) |
| `nodes` | Nodes with the most issues | the 5 nodes with the most active issues (the issue is about the node, or its pods run there, from the live inventory), with how many are urgent; each links to `/issues?status=active&node=` |
| `causes` | Top causes this week, in "This week" | the 5 failure families with the most issues kwatch holds (it keeps a week), counted as `/issues?status=all&mode=` lists them (active quiet issues left out), with a plain label ("Image pull failures", "Out of memory"; other families split into words) and how many are still active |
| `namespaces` | Namespaces with active issues | one tile per namespace with active issues: its most urgent severity and count, most urgent first, then most issues, then A to Z; at most 60 (`more` counts the rest). Each tile is a link to `/issues?status=active&namespace=`, in a list, with the count and severity in words, not color alone. Cluster-wide issues have no tile |
| `quiet` | the quiet line | only when nothing is active: `objects` (the inventory's size), `namespaces` and `kinds` watched (coverage) and `lastScan`, with the scope from `/api/status` in the Issues page's words: "Watching every namespace (34 namespaces now) · 2,841 objects · last checked 10 s ago · What kwatch is watching" |

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
- `ping` every 25 s.

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
no longer valid (checked at every heartbeat) and after 72 heartbeats
(30 minutes), after which the browser reconnects. Each write has its
own 10 s deadline, so the server's write timeout doesn't cut the
stream. The Content Security Policy needs no change: `connect-src
'self'` allows the same-origin `EventSource`.

The page (`js/refresh.js`) opens one stream per tab, closes it while
the tab is hidden and opens it again when it is shown. On `changed`,
every view's reader runs (at most once per 5 s; changes that come
faster are read together): the table page, the Overview's cards and
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
while one is out (it runs once more after it). Every stream message counts as a successful read for the
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
- **Behind a trusted proxy** (`dashboard.trustProxy`), the client is
  the address the proxy appended to `X-Forwarded-For` (a value that is
  not an IP address is ignored and the peer is limited instead), and
  only that client is limited: guesses from some clients never lock out the
  others. Failures through one proxy are counted only to log a warning
  ("many dashboard sign-in failures through one proxy") when 100 come
  in a minute. Without `trustProxy` behind an Ingress every client
  shares the proxy's address and its one limit.
- **Shared mode** (`dashboard.shared`) refuses sign-in over plain http
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
  UI does not have answers `404` with a small HTML page linking to the
  Overview; `/api/*` and missing assets keep the JSON error.
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
freshness line never says "Live" then: it says "Last updated 2 min
ago" and keeps ticking (the bar's Try now is the only one). The
Overview's headline says "Can't reach kwatch — last known at 19:44"
instead of "All clear" or the counts' sentence, and its health line
says it can't tell whether alerts are getting out. The next answer
that works hides the bar.
`401` is not a failure: it goes to sign-in as before. A request the
page cancelled itself (a newer search) counts for nothing.

## Browser notifications

The **Notifications** switch in the sidebar (`js/notify.js`) asks the
browser for permission and, once allowed, shows a system notification when
an issue changes. The settings live in this browser only (`localStorage`
key `kwatch.notify`):

- **Tell me about**: Urgent only, Urgent and needs attention (default) or
  Everything. Quiet issues never notify.
- **Also when resolved**: off by default.
- **Even when this tab is open**: off by default; otherwise a notification
  shows only while the tab is hidden or the window is not focused.

How it works: while a signed-in kwatch tab is open, it reads
`/api/incidents?status=active&sort=last&pageSize=100` and
`/api/incidents?status=resolved&since=24h&sort=last&pageSize=100` (latest
activity first, so what just changed is always among the 100 read, on
any size of cluster) every 30 s, also in the background (browsers slow
a background tab's timer to about once a minute). Each read sends the
list's `ETag`; an unchanged list answers `304` and the last copy is
used again. Each check is compared with the previous one
(`KW.notifyLogic`, pure functions):

- **new**: an issue that was not listed before;
- **reopened**: an issue that was resolved and is active again;
- **escalated**: the same issue at a more urgent severity;
- **resolved**: an active issue that is now resolved (only when asked for).

Nothing is notified for what was already there when the page opened. Each
notification is tagged with the issue ID, so a later one for the same
issue replaces it; its title is "Urgent: <title>" and its body
"namespace/object · what changed". More than three changes in one check
become a single "5 issues changed" notification. Clicking a notification
focuses the tab and opens the issue (or the Issues table for the
summary).

While the tab is hidden, the tab title counts the changes not seen yet,
"(3) Issues · kwatch"; the count clears when the tab is shown. The
count follows the same severity choice, whether or not notifications are
on.

Limits:

- It works only while a kwatch tab is open. There is no service worker;
  the live stream (see "Live push") only makes a check happen sooner
  (at most every 10 s), and only while the tab is shown. Close every
  kwatch tab and nothing arrives. Use the alert
  channels for anything that must reach someone.
- Browsers allow notifications only on a secure origin (https or
  `localhost`); over plain http the switch says so.
- If the browser blocks notifications for the site, the switch explains
  how to allow them; a browser without the Notification API says so.
- An issue that fails and recovers between two checks is not seen.
- The Notification API needs no Content Security Policy change: the page
  still loads nothing from elsewhere and connects only to its own origin.
