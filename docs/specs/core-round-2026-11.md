# Core round 2026-11: message copy and dashboard spec

Owner: kwatch-designer. For: kwatch-core. No production code. Not committed.
Sources: `kwatch.dev/packages/brand/MESSAGING.md`, `internal/notification/compose/testdata/*.golden`, `internal/dashboard/UI_NOTES.md` ("Look and brand", "Issue summary", "Issue page"), `internal/scenarios/SCORECARD.md`.

## 0. Rules for every item

**Alert text.** Terse, technical, cause first, then the fix command. No glossing of terms in messages; seniors must not feel talked down to. No forecast or prediction words ("will", "likely return", "may", "expected to"). Say what kwatch saw, with a number or a time. Generic Kubernetes terms only; no third-party tool names. Emoji in messages are the severity set only (🔴 🟠 ✅), as in the goldens.

**Golden shape.** Each message is the long text, a blank line, then the short text (the headline the incident title uses, max 140 chars). The long text is: headline sentence, one fact sentence, optional second fact, then `To <purpose>, run <command>`. At most 4 sentences before the command. Quoted log lines are cut to 120 characters, exactly as seen.

**Budget.** None of this adds a message. Every item is wording inside the first message or an existing update/resolve (SCORECARD: p95 <= 3 messages per incident, most <= 5, 0 scenarios over budget). New facts that change the cause or the impact ride on the next update only if the headline changes; otherwise they appear on the dashboard only. Nothing here may raise the message count of an existing scenario; kwatch-core to check with `make scorecard` and regenerate goldens.

**Dashboard.** Beginner help is one click away and never in the alert. Per "Look and brand": information is visible text, never only a tooltip. So each new term has (a) the plain visible text, (b) a hover/focus tooltip with a one-sentence meaning, and (c) a folded "What does this mean?" disclosure on the Why row that repeats the tooltips in full, closed by default, state kept in `localStorage` key `kwatch.issueHelp`. Tooltip text follows the MESSAGING.md glossary (pod, node, namespace, ...); link `docs.kwatch.dev/glossary#<anchor>` when one exists. No Signal colour anywhere in these treatments; status colour only inside the existing severity chip. Dark mode, text scale and high-contrast follow the existing tokens; no new tokens. All new marks are text plus an existing line icon, never colour alone; tooltips open on focus as well as hover and are reachable by keyboard (Enter on the term opens its disclosure).

---

## 1. Dependency host read from a ConfigMap

Cause line names the endpoint and where kwatch read it from.

Format: `<endpoint> (from ConfigMap <name>, <KEY>)`. One source only; if the same host is in several keys, name the first and add `, +2 more keys` on the dashboard only. Secrets: the host is shown only if kwatch may read it and it is not redacted; otherwise the clause is omitted (never `from Secret ...` with a value).

Before (`external_endpoint_storm.golden`):
```
🔴 api in shop is crash looping because external endpoint db.example.com:5432 refuses connections. It fails with "dial tcp db.example.com:5432: connect: connection refused". ...
```
After:
```
🔴 api in shop is crash looping because external endpoint db.example.com:5432 (from ConfigMap api-config, DB_HOST) refuses connections. It fails with "dial tcp db.example.com:5432: connect: connection refused". Every error mentions it. Five failing workloads all depend on it. Affected: cart, search, auth and one other. To read the output of the last crash, run kubectl logs api-6d4f-0 -c app -n shop --previous

🔴 api in shop is crash looping because external endpoint db.example.com:5432 (from ConfigMap api-config, DB_HOST) refuses connections.
```
Note: the short form carries the clause too (it is the title; <= 140 chars, so the clause is dropped from the title only when it would cut the endpoint).

Dashboard: the Why row text is the same sentence; `(from ConfigMap api-config, DB_HOST)` is a link to the Evidence tab's Affected objects row for that ConfigMap. Proof gets a new kind `Config` (existing icon set, the "Change" icon is not reused): `DB_HOST in api-config`, detail = `DB_HOST=db.example.com:5432` (monospace, Copy). Tooltip on "ConfigMap": "A Kubernetes object that holds settings for pods, such as a hostname." Folded help: "kwatch found this address in the pod's settings, then saw the connection to it fail."

## 2. Errors while Kubernetes says healthy

A pod Running and Ready whose logs show an error burst new against its own baseline. Severity 🟠 by default.

Headline: `<workload> in <ns> is up but failing: <N> errors in <M> min`. Quoted line is the most frequent error line, with its count, cut to 120 chars. The fact line says what Kubernetes reports. No cause is guessed unless one is found; if one is, it follows as "Likely cause: ...".

Before: no message (this state raises none today).
After:
```
🟠 orders in shop is up but failing: 340 errors in 5 min. Pods are Running and Ready; the log shows "pq: connection refused" 312 times, up from 2 in the previous hour. To read the recent errors, run kubectl logs deploy/orders -n shop --since=5m | grep -i error

🟠 orders in shop is up but failing: 340 errors in 5 min.
```
Rules: the baseline phrase is `up from <n> in the previous hour` (use the real number; if no baseline yet, say `no earlier errors seen`). Count uses "errors", not "exceptions". Not shown below the detection floor; exact threshold is kwatch-core's, and must be stated in the docs page. Core to verify "previous hour" matches the real baseline window; reword to match.

Dashboard: What's wrong reads the headline sentence. The state chip stays at the severity; Proof shows `Log ×312 "pq: connection refused"` first, then a new `Health` proof: `Kubernetes: Running, Ready 3/3`. Tooltip on "up but failing": "Kubernetes sees the pod as running and ready, but its own log is reporting errors. The platform check passes; the application is not working." Evidence tab > Logs gets a small count strip, "errors per minute, last 30 min" as a table with a text min/max, no chart colour beyond `--text`; the baseline is a dashed rule labelled "usual".

## 3. Resolve when the stated cause is still present

Two forms. Never claims a future. The requested line "this will likely return" is a prediction and is not used; the factual replacement is below. Lead: confirm.

3a. Healthy, cause still present (e.g. cause was a config pointing at a dead host, restart cleared the crash):
```
✅ payments in shop is healthy again after a restart, but db.example.com:5432 still refuses connections. The same cause is still present; kwatch will keep watching it. To check it, run kubectl logs payments-6d4f-0 -c app -n shop

✅ payments in shop is healthy again after a restart, but db.example.com:5432 still refuses connections.
```
"kwatch will keep watching it" is allowed only if the code does keep the incident or a follow-up open (core to verify; else delete the clause). Form: `<what recovered> after <action>, but <cause> still <state>.` `<state>` is the present tense observed state ("refuses connections", "is full at 98%").

3b. Cause confirmed recovered (plain resolve, as today):
```
✅ payments in shop is healthy again. db.example.com:5432 accepts connections again. It was failing for 38 minutes.

✅ payments in shop is healthy again.
```
Before: `✅ payments in shop is healthy again. It was failing for 38 minutes.` (resolved_without_fix.golden). The new sentence appears only when the cause was named and its recovery was observed.

Dashboard: header stays "lasted 38 min, resolved 14:02". Stepper "Resolved" gets a second line when 3a: "Resolved, cause still present" and the Why row keeps the cause with a `Still present` label in `--text` (text, not colour); 3b shows `Recovered 14:02` on the Why row. Tooltip on "cause still present": "The app is healthy, but the thing that was breaking it has not recovered. kwatch checked again at resolve time." Alerts tab "What we sent" shows the exact text.

## 4. Same-time correlation

Two workloads that started failing seconds apart and share an upstream. Joined cause names the shared upstream once; the "started together" fact gives the gap in seconds.

```
🔴 cart and search in shop are crash looping because external endpoint cache.example.com:6379 refuses connections. They started failing 4 seconds apart, at 14:02:11 and 14:02:15 UTC. Both read it from ConfigMap shared-config, CACHE_HOST. To read the output of the last crash, run kubectl logs cart-6d4f-0 -c app -n shop --previous

🔴 cart and search in shop are crash looping because external endpoint cache.example.com:6379 refuses connections.
```
Rules: "started failing N seconds apart" only when the gap is <= 60 s (else "within 5 minutes"). With 3+ workloads, follow the existing storm wording ("api in shop and four other workloads ...") and keep the one fact: `All started failing within 12 seconds.` Never imply the shared upstream is proven: if only the timing links them, say `They started failing 4 seconds apart and share <upstream>.` and do not write "because".

Before: two incidents or the storm wording without timing (`shared_signature_storm.golden`).

Dashboard: Impact tab > Other issues keeps "Started at the same time" (UI_NOTES `sameTime`); add a header line there: "cart and search started 4 s apart" with a `same upstream` relation next to `same node`/`same namespace`. Tooltip on "same upstream": "Both depend on the same external address or service, so one failure explains both." Joined issues show Caused by/Moved into as today.

## 5. User endpoints in impact

Complement, do not replace: the object line stays (seniors read it), then the user-facing sentence follows. Hosts and paths come from Ingress or Gateway rules that route to the failing workload; paths only when a rule has a specific path.

Before: `... ingress storefront can't serve traffic.`
After:
```
🔴 checkout in shop is not serving traffic: Service checkout has 0 of 3 endpoints ready. Ingress storefront can't serve it: users can't reach shop.example.com/checkout and 2 more. To check the endpoints, run kubectl get endpoints checkout -n shop
```
Cap in messages: 2 endpoints named, then `and N more` (`and 1 more`, `and 5 more`). Format `host/path`; root path is the bare host; wildcard hosts shown as written. Sort: more specific path first. Wording is `can't reach` for 0 ready endpoints, `may be slow or failing` is not used; for partial loss (some ready) use `<n> of <m> endpoints ready for shop.example.com/checkout`. If no Ingress or Gateway is found, the sentence is omitted, not guessed. Never show query strings or credentials in hosts.

Dashboard: Impact tab > Traffic impact table gains a "Users reach it at" column: up to 8 `host/path` rows (existing bound of 8), not-ready first, each with ready/total endpoints and a Copy; the rest "+N more" opens the Affected objects list. Summary card notes row shows "Users: shop.example.com/checkout +2" (truncated to one line, full in tooltip). Tooltip on "Ingress": "Routes outside web traffic to Services inside the cluster." (docs glossary: add `ingress` anchor, core to tell kwatch-docs).

## 6. Cascade timeline

A compact one-line chain of what failed in order.

Format: `14:02 n3 disk pressure → 14:03 redis evicted → 14:03 checkout, cart crash`. Times hh:mm UTC; same-minute steps share a minute; workloads in the same step joined by `, ` (max 2 names then `+N`); steps separated by ` → `. The verb is the observed event only (`disk pressure`, `evicted`, `crash`, `no ready endpoints`); no "caused", no "led to".

In messages: only for chains of 3 or more steps, and only in the first message that has the whole chain or in the update that completes it. Never in the short form; never in a resolve. One line, placed after the cause sentence and before the command, prefixed `Timeline:`. If it would push the long text past 600 characters, drop it from the message (dashboard keeps it).

```
🔴 checkout and cart in shop are crash looping because redis in shop was evicted from node n3 (disk pressure). Timeline: 14:02 n3 disk pressure → 14:03 redis evicted → 14:03 checkout, cart crash. To see the node's conditions, run kubectl describe node n3

🔴 checkout and cart in shop are crash looping because redis in shop was evicted from node n3 (disk pressure).
```
Chains of 2 steps: the cause sentence already says it; no timeline line.

Dashboard: Timeline tab, above the states strip, the same line as a read-only list ("Chain of events"), one row per step: time, object link, observed event; each row links to its issue or object; steps without a time shown as `time unknown`. The line is not collapsed (visible text); the Why row's cause links down to it. Tooltip on the arrow: "Order in which each step was first seen failing. It shows sequence, not proof of cause." Keys: the existing `t` opens it. Narrow screens: rows stack, never horizontal scroll. Screen readers read it as an ordered list, "step 1 of 3".

## 7. Close-cause probe result

When two candidate causes score within the close-call margin and kwatch ran a read-only check to separate them, say what it checked and what it found, in the same sentence that names the cause. Only checks kwatch actually ran; core to confirm which checks exist (DNS lookup, TCP connect, readiness call, image reachability) and list them in the code comment next to the wording.

```
🟠 api in shop is crash looping because external endpoint db.example.com:5432 refuses connections. kwatch checked: DNS resolves db.example.com fine, so DNS is not the cause. To read the output of the last crash, run kubectl logs api-6d4f-0 -c app -n shop --previous
```
Pattern: `kwatch checked: <check result>, so <rejected cause> is not the cause.` Negative result variant: `kwatch checked: db.example.com does not resolve.` (this becomes the cause line, and no `so` clause is added). Not used when the check was inconclusive (timeout): `kwatch checked db.example.com: no answer in 5 seconds.` State the fact, no verdict. Short form unchanged.

Before: cause line only, no mention that an alternative existed (`unknown_cause_unclear.golden` states uncertainty instead).

Dashboard: Why row confidence meter is raised only if the check changed the score; Proof gets a new kind `Check` (icon: existing "Probe"), text `kwatch checked: DNS resolves db fine`, detail the raw result (monospace, Copy), time. Details tab > "Why kwatch thinks this" lists the two candidates as a short table: cause, ruled out or kept, by what. Tooltip on "kwatch checked": "A read-only test kwatch ran from inside the cluster to tell two possible causes apart. It does not change anything."

## 8. Change picked by blast overlap

When several changes precede the failure, the one chosen is the one whose objects the failing pods share. Give the overlap as a fraction.

```
🟠 orders is failing in shop, likely caused by the 14:01 UTC change to config map orders-config. bob changed key DB_HOST from db-1 to db-2. 9 of 10 failing pods read it. It changed about two minutes before the failure. To check the object the pods depend on, run kubectl get configmap orders-config -n shop -o yaml

🟠 orders is failing in shop, likely caused by the 14:01 UTC change to config map orders-config.
```
Before: `likely_config_change.golden` without the overlap sentence. Rules: the sentence goes right after the change description; `N of M failing pods read it` (or `use it` for a Secret, `mount it` for a volume); shown only when M >= 2 and N/M >= 0.5, else omitted. Pod counts are the numbers at the time the message is composed. "likely caused" stays: the golden's existing hedge is a statement of confidence, not a forecast. Secret values are never printed (key names only, as today).

Dashboard: the "Likely cause" mark in What changed just before gets `9 of 10 failing pods read it` as visible text; a pod-by-pod list ("Read it: 9, did not: 1") sits in a folded row under the change (default closed), each pod linking to Evidence > Containers. Tooltip on the fraction: "How many of the failing pods use this object. A high share means the change touches most of the failure." Winning vs rejected changes are listed in Details > Why kwatch thinks this.

---

## 9. Placement summary (Issue page)

| Item | Summary card | Tab |
|:--|:--|:--|
| 1 | Why sentence, Proof `Config` | Evidence > Affected objects |
| 2 | What's wrong, Proof `Log`, `Health` | Evidence > Logs |
| 3 | Stepper line, Why `Still present` / `Recovered` | Alerts > What we sent |
| 4 | Notes row: "started 4 s apart" | Impact > Other issues |
| 5 | Notes row: `Users: host/path +N` | Impact > Traffic impact |
| 6 | Why links to chain | Timeline > Chain of events |
| 7 | Proof `Check` | Details > Why kwatch thinks this |
| 8 | Likely cause mark with fraction | Timeline > What changed just before |

New hover/focus terms (all repeated in the folded "What does this mean?" under Why): ConfigMap, up but failing, cause still present, same upstream, Ingress, chain of events, kwatch checked, N of M failing pods read it. Add the same terms to `docs.kwatch.dev/glossary` (anchors `configmap`, `ingress`; kwatch-docs).

## 10. Open items for kwatch-core and the lead

1. Item 3: the requested "this will likely return" is a forecast; spec uses the factual form. Lead to confirm.
2. Check against code before shipping: the baseline window in item 2 ("previous hour"); that kwatch keeps watching after 3a; which probes exist for item 7; that Gateway routes are read for item 5.
3. Scorecard: rerun message budget; no scenario may gain a message. Regenerate goldens for 1, 4, 5, 6, 8; add new goldens for 2, 3a, 7.
4. Copy review: any wording change goes back to kwatch-designer; Pro and edition language is untouched here.
5. Accessibility: tooltips duplicate visible text, open on focus, dismiss with Escape; timeline is an ordered list; no colour-only state; contrast per existing tokens in light, dark and high-contrast.
