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
      // level and title follow the channels (KW.setPager).
      page: { label: "Urgent", level: "Sent as urgent",
        title: "Urgent: kwatch sent it as urgent. Look now." },
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
      told: "Who was alerted?",
      // What should I do? without steps of its own.
      noSteps: "kwatch has no fix steps for this yet. Start with what " +
        "Kubernetes reports:" },
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
  /* The heartbeat (the API's kwatch.heartbeat), by state: whether
     anyone is told when kwatch itself stops. Plain words; the
     Community words never ask for a setting it cannot use. */
  KW.TERMS.heartbeat = {
    label: "Heartbeat",
    ok: "Pinging",
    waiting: "Set up; the first ping goes out once kwatch is ready.",
    rejected: "The monitoring URL refused the ping",
    unreachable: "The monitoring URL did not answer",
    not_configured: "Not set up. Nobody is told when kwatch itself stops. Set heartbeatMonitor.url to a monitoring URL that alerts when pings stop.",
    pro_only: "Not part of this edition. To learn when kwatch stops, watch its pod from outside, for example an alert when the kwatch Deployment has no ready replica."
  };

  /* kwatch's own node carries a spot or preemptible label (the API's
     kwatch.spotNode). Plain words, optional next steps, no provider. */
  KW.TERMS.spot = {
    head: "kwatch runs on a spot node",
    text: "A spot node is one the cloud can take back at any time. Each time, kwatch is not watching for a few minutes.",
    next: "Optional: give kwatch a priority class (the chart's priorityClass.create), and if your cluster has stable nodes, set nodeSelector or affinity to keep kwatch on them."
  };

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
    // alertedOnCall and noPagerHint follow the channels (KW.setPager).
    alertedOnCall: "Sent as urgent",
    alertedOnCallSub: "urgent issues opened",
    noPagerHint: "",
    medianResolve: "Median time to resolve",
    /* The week's numbers count the issues someone was alerted about. */
    alerted: "Alerted",
    alertedSub: "issues people were told about",
    /* Resolved counts resolves in the week, so it can be more than
       Alerted: some of them opened the week before. */
    resolvedAlerted: "Resolved this week",
    resolvedAlertedSub: "alerted issues, incl. ones opened earlier",
    medianResolveAlertedSub: "over the alerted issues resolved this week",
    medianResolveAlerted: "Median time to resolve an alerted issue",
    /* changes(24): the title of the Overview's releases block. */
    changes: function (hours) {
      return "Bad releases (last " + hours + " h) and changes behind active issues";
    }
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

  /* Urgent wording follows the channels kwatch sends to. Only a
     pager-kind channel (PagerDuty, Opsgenie ...) reaches an on-call
     person; with chat alone an urgent issue is only sent as urgent.
     Until /api/status says (KW.setPager), the words claim no pager. */
  var URGENT = {
    pager: { level: "Alerted the on-call person",
      title: "Urgent: kwatch alerted the on-call person. Look now.",
      chart: "alerted the on-call person", column: "Alerted on-call",
      legend: "Of those, alerted the on-call person", hint: "" },
    none: { level: "Sent as urgent",
      title: "Urgent: kwatch sent it as urgent. Look now.",
      chart: "sent as urgent", column: "Sent as urgent",
      legend: "Of those, sent as urgent",
      hint: "No pager is connected, so urgent issues go to chat only." }
  };
  KW.TERMS.urgent = URGENT.none;

  /* KW.hasPager: whether any alert channel is a pager, from the kinds
     the server reports (perChannelHealth and the kwatch part's
     delivery). */
  KW.hasPager = function (status, kw) {
    var list = ((status && status.perChannelHealth) || []).concat(
      ((kw || (status && status.kwatch) || {}).delivery) || []);
    return list.some(function (c) { return c && c.kind === "pager"; });
  };

  /* KW.setPager sets the urgent words for whether a pager is
     connected, in the terms, the severity table and the help legend. */
  KW.setPager = function (has) {
    var u = URGENT[has ? "pager" : "none"], T = KW.TERMS;
    T.urgent = u;
    T.severity.page.level = T.lists.alertedOnCall = u.level;
    T.severity.page.title = u.title;
    T.lists.noPagerHint = u.hint;
    if (KW.SEVERITY) {
      KW.SEVERITY.page.level = u.level;
      KW.SEVERITY.page.title = u.title;
    }
    Array.prototype.forEach.call(document.querySelectorAll("[data-legend-urgent]"),
      function (e) { e.textContent = u.title.replace(/^Urgent: /, ""); });
  };
})();
