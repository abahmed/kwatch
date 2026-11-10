/* kwatch dashboard: the Activity page (/activity), what changed
   recently: from /api/activity, the issues that are new, got more
   urgent, came back or were resolved in the last hour (or 24 hours,
   ?since=24h), newest first, at most what the server sends, each a
   link to its issue page. One line counts them by kind. Changes nobody
   was told about (an issue that settled on its own, and its
   resolve) are left out unless the reader shows them (?untold=1); a
   link says how many there are. It reads the feed with its ETag, so
   an unchanged feed costs a 304. */
(function () {
  "use strict";
  var KW = window.KW;
  var T = KW.TERMS.activity;
  var ORDER = ["new", "urgent", "reopened", "resolved"];
  var WINDOWS = { "1h": "the last hour", "24h": "the last 24 hours" };
  /* TALLY: each kind counted in words, "25 issues started". */
  var TALLY = {
    "new": function (n) { return KW.num(n) + (n === 1 ? " issue" : " issues") + " started"; },
    urgent: function (n) { return KW.num(n) + " got more urgent"; },
    reopened: function (n) { return KW.num(n) + " came back"; },
    resolved: function (n) { return KW.num(n) + " resolved"; }
  };
  /* A burst: this many rows or more at one minute, of one kind, show
     the first SHOWN and fold the rest. */
  var BURST = 4, SHOWN = 2;

  function lower(s) { return s.charAt(0).toLowerCase() + s.slice(1); }

  /* who: "api in shop", "datadog-webhook in the whole cluster". */
  function who(e) {
    var name = e.workload || KW.shortTitle(e.title, e);
    return name + " in " + (e.namespace || "the whole cluster");
  }

  /* verb: what happened, the end of the sentence. */
  function verb(e) {
    if (e.kind === "resolved") {
      if (e.untold) return T.settled;
      var from = KW.now(), took = e.lasted > 0 ? KW.lasted(new Date(from).toISOString(),
        new Date(from + e.lasted * 1000).toISOString()) : "";
      return T.resolved + (took ? " after " + took : "");
    }
    if (e.kind === "urgent") {
      return T.urgent + " (" + lower(KW.TERMS.severity.page.level) + ")";
    }
    var say = T[e.kind] || e.kind;
    return e.untold ? say + "; " + lower(KW.TERMS.notAlerted) : say;
  }

  /* row: time, one sentence saying what happened to which workload,
     the issue's own title under it, and how urgent it is. */
  function row(e) {
    var li = KW.el("li");
    var a = li.appendChild(KW.el("a", "act-row"));
    a.href = "/issues/" + encodeURIComponent(e.id);
    a.dataset.link = "";
    a.dataset.rowlink = "";
    a.appendChild(KW.hm(e.at)).className = "act-at";
    var body = a.appendChild(KW.el("span", "act-body"));
    var line = body.appendChild(KW.el("span", "act-line"));
    line.appendChild(KW.el("b", "act-who", who(e)));
    line.appendChild(KW.el("span", "act-verb", ": " + verb(e)));
    var title = KW.shortTitle(e.title, e);
    if (title && title !== e.workload) {
      KW.clampTitle(body.appendChild(KW.el("span", "act-title", title)));
    }
    var chips = a.appendChild(KW.el("span", "act-chips"));
    chips.appendChild(e.kind === "resolved" && e.untold ? KW.wasChip(e.severity) :
      KW.severityChip(e.severity, e.kind === "resolved" ? "Was: " : ""));
    return li;
  }

  /* fold: the rest of a burst behind one line, "6 more resolved at
     14:55". open says which folds the reader has open. */
  function fold(rows, e, open) {
    var li = KW.el("li", "act-burst");
    var d = li.appendChild(KW.el("details", "act-fold"));
    var at = KW.hm(e.at).textContent;
    var key = at + " " + e.kind;
    d.dataset.burst = key;
    d.open = !!open[key];
    d.addEventListener("toggle", function () { open[key] = d.open; });
    d.appendChild(KW.el("summary", "", KW.num(rows.length) + " more " + T[e.kind] +
      " at " + at));
    var ul = d.appendChild(KW.el("ul", "act-sub"));
    rows.forEach(function (x) { ul.appendChild(row(x)); });
    return li;
  }

  /* drawRows puts events in list, folding bursts. */
  function drawRows(list, events, open) {
    var i = 0;
    while (i < events.length) {
      var e = events[i], key = KW.hm(e.at).textContent + e.kind, j = i;
      while (j < events.length && events[j].kind === e.kind &&
        KW.hm(events[j].at).textContent + events[j].kind === key) j++;
      var n = j - i, shown = n >= BURST ? SHOWN : n;
      for (var k = i; k < i + shown; k++) list.appendChild(row(events[k]));
      if (n > shown) list.appendChild(fold(events.slice(i + shown, j), e, open));
      i = j;
    }
  }

  /* tally: "25 issues started, 4 got more urgent and 21 resolved", from
     the server's counts for the whole window (kinds), else from the
     events shown. */
  function tally(events, kinds) {
    var n = kinds || {};
    if (!kinds) events.forEach(function (e) { n[e.kind] = (n[e.kind] || 0) + 1; });
    var parts = ORDER.filter(function (k) { return n[k]; }).map(function (k) {
      return TALLY[k](n[k]);
    });
    if (parts.length < 2) return parts.join("");
    return parts.slice(0, -1).join(", ") + " and " + parts[parts.length - 1];
  }

  // STEP events at first; "Show more" adds twice as many, up to MAX
  // (the API's limit).
  var STEP = 20, MAX = 200;

  KW.activityView = function (main) {
    var query = new URLSearchParams(location.search);
    var since = query.get("since") === "24h" ? "24h" : "1h";
    var untold = query.get("untold") === "1";
    var etag = "", seq = 0, limit = STEP, open = {};
    var pg = KW.pageView(main, { title: "Activity", tab: "Activity · kwatch", read: load,
      sub: "What changed recently: issues that started, got more urgent, came back or were resolved. Newest first." });
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
    var moreText = more.appendChild(KW.el("span"));
    var moreB = more.appendChild(KW.el("button", "btn btn-sm act-moreb", "Show more"));
    moreB.type = "button";
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
      drawRows(list, events, open);
      list.hidden = !events.length;
      line.classList.toggle("is-empty", !events.length);
      var total = data.total || events.length;
      line.textContent = events.length ? "In " + WINDOWS[since] + ", " +
        tally(events, data.kinds) + "." : "Nothing changed in " + WINDOWS[since] + ".";
      more.hidden = total <= events.length;
      moreText.textContent = "Showing the latest " + KW.num(events.length) + " of " +
        KW.num(total) + " changes. ";
      moreB.hidden = limit >= MAX;
      var n = data.untold || 0;
      quiet.hidden = !n;
      quietB.textContent = untold ? "Hide changes nobody was alerted about" :
        "Also show " + KW.plural(n, "change", "changes") + " nobody was alerted about";
      quietB.title = "Issues that settled on their own before kwatch alerted anyone.";
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
      return KW.api("/api/activity?since=" + since + (untold ? "&untold=1" : "") +
        (limit !== STEP ? "&limit=" + limit : ""),
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
        limit = STEP;
        mark();
        place();
        pg.now();
      });
    });
    moreB.addEventListener("click", function () {
      limit = Math.min(MAX, limit + STEP * 2);
      etag = "";
      pg.now();
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
