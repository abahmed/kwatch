/* kwatch dashboard: "What changed recently" on the Overview. A short
   feed from /api/activity of the last hour (or 24 hours): issues that
   are new, got more urgent, came back or were resolved, newest first,
   at most 20, each a link to its issue page. It reads the feed with its
   ETag, so an unchanged feed costs a 304. */
(function () {
  "use strict";
  var KW = window.KW;
  var KINDS = {
    "new": ["New", "c-state", "It started failing."],
    urgent: ["More urgent", "c-page", "It kept failing, so kwatch alerted the on-call person."],
    reopened: ["Came back", "c-state c-flap", "It failed again after it was resolved."],
    resolved: [KW.STATE.resolved.label, "c-resolved", "It stopped failing."]
  };
  var WINDOWS = { "1h": "the last hour", "24h": "the last 24 hours" };

  function sep(a) { a.appendChild(KW.el("span", "act-sep", "·")); }

  /* row: "12:31 · New · Urgent · svc-204 cannot pull its image · team-6". */
  function row(e) {
    var a = KW.el("a", "act-row");
    a.href = "/issues/" + encodeURIComponent(e.id);
    a.dataset.link = "";
    a.appendChild(KW.hm(e.at)).className = "act-at";
    sep(a);
    var k = KINDS[e.kind] || [e.kind, "c-state", ""];
    a.appendChild(KW.chip(k[1], k[0], k[2]));
    sep(a);
    var sev = KW.SEVERITY[e.severity];
    a.appendChild(KW.el("span", "act-sev", sev ? sev.label : e.severity)).title =
      sev ? sev.title : "";
    sep(a);
    a.appendChild(KW.el("span", "act-title", e.title));
    sep(a);
    a.appendChild(KW.el("span", "act-ns", e.namespace || "whole cluster"));
    var li = KW.el("li");
    li.appendChild(a);
    return li;
  }

  /* activityFeed wires the section of root; load() reads it again. */
  KW.activityFeed = function (root) {
    var box = KW.$("[data-activity]", root);
    var list = KW.$("[data-act-list]", root);
    var none = KW.$("[data-act-none]", root), more = KW.$("[data-act-more]", root);
    var buttons = KW.$$("[data-act-since]", root);
    var since = "1h", etag = "", stopped = false, seq = 0;

    function draw(data) {
      var events = data.events || [];
      list.textContent = "";
      events.forEach(function (e) { list.appendChild(row(e)); });
      list.hidden = !events.length;
      none.hidden = !!events.length;
      none.textContent = "Nothing changed in " + WINDOWS[since] + ".";
      var total = data.total || events.length;
      more.hidden = total <= events.length;
      more.textContent = "Showing the latest " + KW.num(events.length) + " of " +
        KW.num(total) + " changes.";
      box.hidden = false;
    }

    function load() {
      var mine = ++seq;
      return KW.api("/api/activity?since=" + since, { etag: etag }).then(function (res) {
        if (stopped || mine !== seq || res.status !== 200 || !res.data) return;
        etag = res.etag;
        draw(res.data);
      });
    }

    buttons.forEach(function (b) {
      b.addEventListener("click", function () {
        since = b.dataset.actSince;
        etag = "";
        buttons.forEach(function (x) {
          x.setAttribute("aria-pressed", x === b ? "true" : "false");
        });
        load();
      });
    });

    return { load: load, stop: function () { stopped = true; } };
  };
})();
