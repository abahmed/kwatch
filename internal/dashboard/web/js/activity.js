/* kwatch dashboard: the Activity page (/activity), what changed
   recently: from /api/activity, the issues that are new, got more
   urgent, came back or were resolved in the last hour (or 24 hours,
   ?since=24h), newest first, at most what the server sends, each a
   link to its issue page. One line counts them by kind. Changes nobody
   was told about (an issue that settled before alerting, and its
   resolve) are left out unless the reader shows them (?untold=1); a
   link says how many there are. It reads the feed with its ETag, so
   an unchanged feed costs a 304. */
(function () {
  "use strict";
  var KW = window.KW;
  var KINDS = {
    "new": ["New", "c-state", "It started failing."],
    urgent: ["More urgent", "c-page", "It kept failing, so kwatch alerted the on-call person."],
    reopened: ["Came back", "c-state c-flap", "It failed again after it was resolved."],
    resolved: [KW.STATE.resolved.label, "c-resolved", "It stopped failing."]
  };
  var ORDER = ["new", "urgent", "reopened", "resolved"];
  var WINDOWS = { "1h": "the last hour", "24h": "the last 24 hours" };

  /* row: time, what happened, the issue and where it is. */
  function row(e) {
    var li = KW.el("li");
    var a = li.appendChild(KW.el("a", "act-row"));
    a.href = "/issues/" + encodeURIComponent(e.id);
    a.dataset.link = "";
    a.dataset.rowlink = "";
    a.appendChild(KW.hm(e.at)).className = "act-at";
    var k = KINDS[e.kind] || [e.kind, "c-state", ""];
    var what = a.appendChild(KW.el("span", "act-what"));
    what.appendChild(KW.chip(k[1], k[0], k[2]));
    what.appendChild(KW.severityChip(e.severity, e.kind === "resolved" ? "Was: " : ""));
    if (e.untold) what.appendChild(KW.untoldMark());
    var main = a.appendChild(KW.el("span", "act-main"));
    KW.clampTitle(main.appendChild(KW.el("span", "act-title", e.title)));
    // e.kind is what happened, not the object's kind.
    main.appendChild(KW.el("span", "act-ns", KW.whereText({ namespace: e.namespace,
      workload: e.workload, workloadKind: e.workloadKind })));
    return li;
  }

  function tally(events) {
    var n = {};
    events.forEach(function (e) { n[e.kind] = (n[e.kind] || 0) + 1; });
    return ORDER.filter(function (k) { return n[k]; }).map(function (k) {
      return KW.num(n[k]) + " " + KINDS[k][0].toLowerCase();
    }).join(", ");
  }

  KW.activityView = function (main) {
    var query = new URLSearchParams(location.search);
    var since = query.get("since") === "24h" ? "24h" : "1h";
    var untold = query.get("untold") === "1";
    var etag = "", seq = 0;
    var pg = KW.pageView(main, { title: "Activity", tab: "Activity · kwatch", read: load,
      sub: "What changed recently: issues that started, got more urgent, came back or were resolved." });
    var sw = pg.body.appendChild(KW.el("div", "tz act-sw"));
    sw.setAttribute("role", "group");
    sw.setAttribute("aria-label", "Show changes from");
    var buttons = ["1h", "24h"].map(function (w) {
      var b = sw.appendChild(KW.el("button", "tz-b", w === "1h" ? "Last hour" : "Last 24 hours"));
      b.type = "button";
      b.dataset.actSince = w;
      return b;
    });
    var line = pg.body.appendChild(KW.el("p", "act-sum"));
    var list = pg.body.appendChild(KW.el("ul", "act-list"));
    var more = pg.body.appendChild(KW.el("p", "act-more"));
    var quiet = pg.body.appendChild(KW.el("p", "act-quiet"));
    var quietB = quiet.appendChild(KW.el("button", "link"));
    quietB.type = "button";
    quietB.dataset.actUntold = "";
    pg.body.hidden = true;

    function mark() {
      buttons.forEach(function (b) {
        b.setAttribute("aria-pressed", b.dataset.actSince === since ? "true" : "false");
      });
    }

    function draw(data) {
      var events = data.events || [];
      list.textContent = "";
      events.forEach(function (e) { list.appendChild(row(e)); });
      list.hidden = !events.length;
      line.classList.toggle("is-empty", !events.length);
      var total = data.total || events.length;
      line.textContent = events.length ? KW.plural(total, "change", "changes") + " in " +
        WINDOWS[since] + ": " + tally(events) + "." : "Nothing changed in " + WINDOWS[since] + ".";
      more.hidden = total <= events.length;
      more.textContent = "Showing the latest " + KW.num(events.length) + " of " +
        KW.num(total) + " changes.";
      var n = data.untold || 0;
      quiet.hidden = !n;
      quietB.textContent = untold ? "Hide changes nobody was told about" :
        "Show " + KW.plural(n, "change", "changes") + " nobody was told about";
      quietB.title = "Issues that settled before kwatch alerted anyone, and their resolves.";
      quietB.setAttribute("aria-pressed", untold ? "true" : "false");
    }

    /* place keeps the window and the untold choice in the address. */
    function place() {
      var q = new URLSearchParams();
      if (since === "24h") q.set("since", "24h");
      if (untold) q.set("untold", "1");
      var s = q.toString();
      history.replaceState(null, "", "/activity" + (s ? "?" + s : ""));
      KW.rendered = location.pathname + location.search;
    }

    function load() {
      var mine = ++seq;
      return KW.api("/api/activity?since=" + since + (untold ? "&untold=1" : ""),
        { etag: etag }).then(function (res) {
        if (mine !== seq) return null;
        if (res.status === 200 && res.data) {
          etag = res.etag;
          draw(res.data);
        }
        return res;
      });
    }

    buttons.forEach(function (b) {
      b.addEventListener("click", function () {
        since = b.dataset.actSince;
        etag = "";
        mark();
        place();
        pg.now();
      });
    });
    quietB.addEventListener("click", function () {
      untold = !untold;
      etag = "";
      place();
      pg.now();
    });
    mark();
    return { stop: pg.stop };
  };
})();
