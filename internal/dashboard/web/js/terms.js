/* kwatch dashboard: the words every page uses for the same thing, in
   one place, so a page never invents its own. Loaded before core.js.
   Severity words are Urgent, Needs attention and Low (and Quiet for an
   issue kwatch only tracks); the UI never says page, notify or digest.
   "Alerted" is what kwatch did to a channel (Slack, PagerDuty). */
(function () {
  "use strict";
  var KW = (window.KW = window.KW || {});

  KW.TERMS = {
    /* The alert level of an issue: the badge, how people were alerted
       (level) and the tooltip (title). The keys are the API's. */
    severity: {
      page: { label: "Urgent", level: "Alerted the on-call person",
        title: "Urgent: kwatch alerted the on-call person. Look now." },
      notify: { label: "Needs attention", level: "Alerted your chat channels",
        title: "Needs attention: kwatch alerted your chat channels. Look soon." },
      digest: { label: "Low", level: "Listed in the daily summary only",
        title: "Low: listed in the daily summary only. No rush." },
      silent: { label: "Quiet", level: "Not alerted",
        title: "Quiet: nobody was alerted. kwatch only keeps track of it." }
    },
    /* An issue that recovered before kwatch had to alert anyone. */
    settled: { label: "Settled on its own",
      line: "Settled on its own: nobody was alerted.",
      title: "It recovered while kwatch was still checking it, so nobody was alerted.",
      wasTitle: "It reached this level, but recovered while kwatch was still checking it, so nobody was alerted." },
    /* How an old occurrence ended (the API's fix). */
    fix: {
      rollback: "fixed by a rollback",
      "config fix": "fixed by a config change",
      "node replaced": "the node was replaced",
      change: "fixed by a change",
      "recovered without change": "settled on its own"
    },
    alerted: "Alerted",
    notAlerted: "Nobody was alerted",
    proof: "Proof",
    browserAlerts: "Browser alerts",
    navHealth: "kwatch health",
    /* The questions at the top of an issue page. */
    ask: { what: "What's wrong?", why: "Why?", todo: "What should I do?",
      told: "Who was alerted?" },
    /* A bundled message: not sent on its own. */
    bundled: "bundled in the summary message"
  };

  /* The Activity page: what happened to an issue, as the end of the
     sentence "api in shop: ...". */
  KW.TERMS.activity = {
    "new": "started failing",
    urgent: "became Urgent",
    reopened: "failed again after it was resolved",
    resolved: "resolved",
    settled: "settled on its own"
  };

  /* The kwatch health page: kwatch's own state in plain words. */
  KW.TERMS.health = {
    ok: "kwatch is working normally",
    kinds: "resource types",
    kindsNote: "Resource types are the kinds of Kubernetes objects, such as Deployments and Pods.",
    api: "Last heard from the Kubernetes API",
    waiting: "Alerts waiting to be sent"
  };

  /* The kwatch health page: an optional part of kwatch that does not
     work (the API's kwatch.blindSpots, by health component and reason):
     what kwatch can't do, then what that means. Words only, no fix
     steps. */
  KW.TERMS.blind = {
    parts: {
      "kubelet-stats": ["kwatch can't read node stats from the kubelets",
        "Memory and CPU use may be missing on issue pages."],
      rbac: ["kwatch is missing some permissions it needs",
        "Some checks can't run, so some problems may go unseen."],
      "config-overlay": ["kwatch's configuration override isn't applied",
        "kwatch runs with its base settings."],
      state: ["kwatch can't save what it knows",
        "Issues and unsent alerts may be lost if kwatch restarts."],
      "state-store": ["kwatch can't save what it knows",
        "Issues and unsent alerts may be lost if kwatch restarts."],
      "state-store-size": ["kwatch's saved history is over its size limit",
        "kwatch drops the oldest history first."],
      "usage-history": ["kwatch isn't keeping usage history",
        "Usage charts on issue pages may be empty."],
      heartbeat: ["kwatch's heartbeat isn't getting out",
        "A watchdog outside the cluster may think kwatch is down."],
      "dashboard-credentials": ["kwatch can't read the dashboard sign-in settings",
        "Signing in to this dashboard may stop working."],
      "dashboard-server": ["The dashboard had trouble serving",
        "This page may stop updating."],
      "delivery-outbox": ["kwatch can't keep alerts that wait to be sent",
        "Alerts that fail to send may be lost if kwatch restarts."],
      "crd-watcher": ["kwatch isn't reading its settings objects",
        "Changes to kwatch's own Kubernetes objects aren't picked up."],
      "leader-election": ["kwatch's leader election isn't working",
        "Another kwatch replica may take over."]
    },
    /* The license, by reason: [head, what it means]. */
    license: {
      license_invalid: ["The license key isn't valid", "Pro features are off."],
      license_expired: ["The Pro license has expired", "Pro features are off."],
      license_revoked: ["The Pro license was revoked", "Pro features are off."],
      license_conflict: ["The license is in use on another cluster", "Pro features are off here."],
      license_pending: ["kwatch is still checking its license", "Pro features wait until it is checked."],
      other: ["The Pro license needs a look", "Pro features may be off."]
    },
    other: "Part of kwatch isn't working ({part})",
    /* Issues not announced yet only because kwatch has not read their
       namespace's labels (the API's kwatch.scopeWaiting): not out of
       scope. {n} is "1 issue is" or "3 issues are". */
    scopeWaiting: ["{n} waiting: kwatch hasn't read their namespace's labels yet",
      "They are announced once kwatch has read them, or left out if their namespace is out of scope."],
    reasons: {
      permission_denied: "kwatch isn't allowed to read it",
      optional_permission_denied: "kwatch isn't allowed to read it",
      kubelet_unreachable: "no kubelet answers",
      kubelet_partially_unreachable: "some kubelets don't answer",
      api_unavailable: "the Kubernetes API didn't answer",
      optional_api_unavailable: "the Kubernetes API doesn't serve it",
      timeout: "it timed out",
      rate_limited: "the Kubernetes API is slowing kwatch down",
      storage_over_cap: "it is over its size limit",
      storage_reset: "the saved history was reset",
      persistence_write_failed: "writing to disk failed",
      persistence_restore_failed: "reading the saved history failed",
      config_overlay_invalid: "the override isn't valid",
      heartbeat_failed: "the last heartbeat failed",
      leadership_lost: "this kwatch lost the lead",
      cache_sync_pending: "it is still starting",
      cache_sync_failed: "its first read didn't finish",
      cache_sync_timeout: "its first read didn't finish",
      component_stopped: "it stopped",
      other: "it isn't working right now"
    }
  };

  /* Overview and Issues list: counts as a phrase ("1 urgent"), the
     order rule and what the week's numbers count. */
  KW.TERMS.lists = {
    counts: { page: "urgent", notify: "need attention", digest: "low",
      silent: "quiet" },
    order: "Most urgent first, then longest running.",
    alertedOnCall: "Alerted the on-call person",
    alertedOnCallSub: "urgent issues opened",
    medianResolve: "Median time to resolve",
    /* The week's numbers count the issues someone was alerted about. */
    alerted: "Alerted",
    alertedSub: "issues people were told about",
    resolvedAlerted: "Resolved",
    resolvedAlertedSub: "of the alerted issues",
    medianResolveAlerted: "Median time to resolve an alerted issue",
    changes: "Bad releases and changes that caused issues"
  };

  /* Paging and grouping: an issue that groups the objects a removed
     controller left behind, old problems kwatch opened no issue for,
     pages held while nodes come and go, a non-production cluster. */
  KW.TERMS.paging = {
    leftTitle: "This issue groups the objects a removed or stopped controller left behind.",
    leftHead: "Objects left behind",
    old: "already failing for a long time when kwatch first saw them",
    oldDays: "already failing for {days}+ days when kwatch first saw them",
    oldHead: "Old problems not shown",
    held: "Paging held while the cluster scales",
    heldTold: "the page is held while the cluster scales",
    nonProd: "Non-production cluster: nothing pages"
  };
})();
