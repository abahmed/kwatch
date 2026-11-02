/* kwatch dashboard: Overview, the landing page: an incident view. First
   what is broken now in plain words, today in one line and the insight
   cards (ovinsights.js: urgent issues nobody acknowledged first), the
   namespaces with active issues, what changed recently (activity.js),
   then "This week" folded (trends.js), then whether kwatch itself is
   working and alerts get out, with the details of that and of what it
   watches (ovcoverage.js) folded away. While kwatch can't be reached it
   says so instead of "All clear". */
(function () {
  "use strict";
  var KW = window.KW;
  var EVERY = 20000;
  var DAY = 24 * 3600 * 1000;
  var HINT = "Check the provider settings in the installer: \"Edit notification providers\".";

  function failingNow(c) {
    if (!KW.valid(c.lastError)) return false;
    return !KW.valid(c.lastSuccess) || Date.parse(c.lastError) > Date.parse(c.lastSuccess);
  }

  function stateLine(cls, icon, text) {
    var s = KW.el("span", "ch-state " + cls);
    s.appendChild(KW.icon(icon));
    s.appendChild(KW.el("span", "", text));
    return s;
  }

  function channel(c) {
    var row = KW.el("div", "ch");
    var top = KW.el("div", "ch-top");
    top.appendChild(KW.el("span", "ch-name", c.name || c.channel));
    row.appendChild(top);
    if (failingNow(c)) {
      top.appendChild(stateLine("bad", "x", "last failed " + KW.rel(c.lastError)));
      if (c.lastErrorMessage) {
        var msg = KW.el("p", "ch-msg clamp", c.lastErrorMessage);
        row.appendChild(msg);
        if (c.lastErrorMessage.length > 160) {
          var more = KW.el("button", "link small", "Show more");
          more.type = "button";
          more.addEventListener("click", function () {
            var open = msg.classList.toggle("clamp");
            more.textContent = open ? "Show more" : "Show less";
          });
          row.appendChild(more);
        }
      }
      var detail = KW.valid(c.lastSuccess) ?
        "Last delivered " + KW.rel(c.lastSuccess) + ". " : "It has not delivered yet. ";
      row.appendChild(KW.el("p", "ch-detail", detail + HINT));
      return row;
    }
    if (KW.valid(c.lastSuccess)) {
      top.appendChild(stateLine("ok", "check", "delivering"));
      var d = KW.el("p", "ch-detail", "Last sent " + KW.rel(c.lastSuccess));
      if (KW.valid(c.lastError) && KW.now() - Date.parse(c.lastError) < DAY) {
        d.appendChild(document.createTextNode(" · Failed once " + KW.rel(c.lastError)));
      }
      row.appendChild(d);
      return row;
    }
    top.appendChild(stateLine("none", "dash", "nothing sent yet"));
    return row;
  }

  /* headline: one plain sentence about the cluster. */
  function headline(counts) {
    var urgent = counts.page || 0;
    var other = (counts.notify || 0) + (counts.digest || 0);
    if (urgent) {
      return KW.plural(urgent, "urgent problem needs", "urgent problems need") +
        " attention";
    }
    if (counts.notify) {
      return KW.plural(counts.notify, "problem needs", "problems need") +
        " attention";
    }
    if (other) return "Nothing urgent. " + KW.plural(other, "low problem", "low problems") + " to look at";
    return "All clear. Nothing is failing";
  }

  /* overview draws the headline and the counts. While kwatch can't be
     reached the headline says so, with the time of the last answer:
     the counts below are from then. */
  function overview(root, counts) {
    var box = KW.$("[data-overview]", root);
    box.hidden = !counts;
    if (!counts) return;
    var h = KW.$("[data-headline]", root);
    h.textContent = headline(counts) + ".";
    h.className = "ov-h" + (counts.page ? " urgent" : "");
    if (KW.down() && KW.lastOk()) {
      h.textContent = "Can't reach kwatch — last known at " +
        KW.hhmm(new Date(KW.lastOk() + KW.skew));
      h.className = "ov-h stale";
    }
    var row = KW.$("[data-counts]", root);
    row.textContent = "";
    ["page", "notify", "digest"].forEach(function (sev) {
      var a = KW.el("a", "ov-n");
      a.href = "/issues?status=active&severity=" + sev;
      a.dataset.link = "";
      a.title = KW.SEVERITY[sev].title;
      a.appendChild(KW.el("span", "dot " + (sev === "digest" ? "low" : sev)));
      a.appendChild(KW.el("b", "", KW.num(counts[sev])));
      a.appendChild(KW.el("span", "", KW.SEVERITY[sev].label));
      row.appendChild(a);
    });
  }

  /* gapLine: the kinds kwatch isn't allowed to read (its role lacks
     permission), from /api/coverage, or "". */
  function gapLine(cov) {
    var kinds = ((cov && cov.kinds && cov.kinds.unreadable) || [])
      .filter(function (g) { return g.reason === "permission_denied"; })
      .map(function (g) { return g.kind; });
    if (!kinds.length) return "";
    var list = kinds.length <= 3 ? kinds.slice(0, -1).join(", ") +
      (kinds.length > 1 ? " and " : "") + kinds[kinds.length - 1] :
      kinds.slice(0, 3).join(", ") + " and " + KW.num(kinds.length - 3) + " more kinds";
    return "kwatch isn't allowed to read " + list +
      ", so it can't see problems there. Its role needs permission.";
  }

  /* summary: is kwatch itself working and are alerts getting out, in
     one line (and a second for what it can't read). Details open by
     themselves when something is wrong. */
  function summary(node, s, cov) {
    var chans = s.perChannelHealth || [];
    var row, gap = gapLine(cov);
    if (KW.down()) {
      row = ["n-degraded", "Can't reach kwatch, so this page can't say whether alerts are getting out."];
    } else if (chans.some(failingNow)) {
      row = ["n-degraded", "kwatch is running, but some alerts aren't getting out."];
    } else if (!KW.valid(s.lastScan)) {
      row = ["", "kwatch is starting and hasn't finished its first check."];
    } else if (s.lastScanLate) {
      row = ["n-degraded", "kwatch hasn't checked your cluster in a while."];
    } else {
      row = ["n-success", "kwatch is working and your alerts are getting out."];
    }
    var bad = row[0] === "n-degraded" && !KW.down();
    if (gap && row[0] === "n-success") row = ["n-degraded", "kwatch is working and your alerts are getting out, but it can't see everything."];
    node.hidden = false;
    node.className = "notice sum " + row[0];
    node.textContent = "";
    node.appendChild(KW.el("b", "", row[1]));
    if (gap && !KW.down()) node.appendChild(KW.el("p", "", gap));
    return bad;
  }

  function fill(root, s, first, cov) {
    if (summary(KW.$("[data-summary]", root), s, cov) && first) {
      KW.$("[data-body]", root).open = true;
    }
    var about = KW.$("[data-about]", root);
    about.textContent = "";
    function add(label, value, cls) {
      about.appendChild(KW.el("dt", "", label));
      var dd = KW.el("dd", cls || "");
      if (typeof value === "string") dd.textContent = value;
      else dd.appendChild(value);
      about.appendChild(dd);
    }
    add("Version", s.version || "unknown", "mono");
    var ed = (s.edition || "").toLowerCase();
    add("Plan", ed === "pro" ? "Pro" : ed === "community" ? "Community" : s.edition || "unknown");
    if (KW.valid(s.lastScan)) {
      var last = KW.el("span");
      last.appendChild(KW.timeEl(s.lastScan, "", true));
      last.appendChild(KW.el("br"));
      last.appendChild(KW.el("span", "muted small", KW.full(s.lastScan)));
      add("Last check", last);
    } else {
      add("Last check", "Not yet");
    }
    var ch = KW.$("[data-channels]", root);
    ch.textContent = "";
    var list = s.perChannelHealth || [];
    if (!list.length) {
      ch.appendChild(KW.el("p", "nochan", "No alert channels are set up yet."));
      ch.appendChild(KW.el("p", "muted small",
        "Add one in the installer: choose \"Edit notification providers\"."));
    }
    var wrap = KW.el("div");
    list.forEach(function (c) { wrap.appendChild(channel(c)); });
    ch.appendChild(wrap);
  }

  KW.statusView = function (main) {
    KW.title("Overview");
    main.appendChild(KW.clone("tpl-status"));
    var body = KW.$("[data-body]", main);
    var alertBox = KW.$("[data-alert]", main);
    var footer = KW.$("[data-updated]", main);
    var lastOk = 0, failing = false, attempt = 0, stopped = false;
    var counts = null, countsTag = "", last = null;
    var scrollBack = KW.takeScroll();
    var trends = KW.trendsView(main);
    var activity = KW.activityFeed(main);
    var insights = KW.insightsView(main);
    var coverage = KW.coverageView(main);

    /* paint draws what depends on whether kwatch can be reached. */
    function paint() {
      overview(main, counts);
      if (last) fill(main, last, false, coverage.data());
      KW.footer(footer, lastOk, failing);
    }

    function readCounts() {
      return KW.api("/api/incidents?status=active&pageSize=25", { etag: countsTag })
        .then(function (res) {
          if (res.status !== 200 || !res.data || !res.data.facets) return;
          countsTag = res.etag;
          counts = res.data.facets.severity || {};
          KW.updateBadge(counts);
        });
    }

    function load() {
      insights.load();
      trends.load();
      activity.load();
      return Promise.all([KW.api("/api/status"), readCounts(),
        coverage.load()]).then(function (out) {
        var res = out[0];
        if (stopped) return 0;
        // Without an answer the last counts stay (refresh.js says so).
        if (counts || !lastOk) overview(main, counts);
        if (KW.authFailed(res)) return 0;
        alertBox.textContent = "";
        if (res.status !== 200 || !res.data) {
          failing = true;
          var k = KW.kindOf(res.status);
          // What was shown stays; the banner (refresh.js) says it is old.
          if (!lastOk) {
            body.hidden = true;
            alertBox.appendChild(KW.panel(k, res.status, function () { poll.now(); }));
          }
          paint();
          attempt += 1;
          return KW.retryDelay(k, attempt);
        }
        failing = false;
        attempt = 0;
        var first = !lastOk;
        lastOk = Date.now();
        last = res.data;
        KW.self = { namespace: res.data.kwatchNamespace || "kwatch",
          deployment: res.data.kwatchDeployment || "kwatch" };
        body.hidden = false;
        fill(main, res.data, first, coverage.data());
        insights.scope(res.data);
        KW.footer(footer, lastOk, false);
        if (scrollBack) {
          window.scrollTo(0, scrollBack);
          scrollBack = 0;
        }
        return 0;
      });
    }
    var poll = KW.poller(load, EVERY);
    var tick = window.setInterval(function () { KW.footer(footer, lastOk, failing); }, 5000);
    document.addEventListener("kw:conn", paint);
    return { stop: function () {
      stopped = true;
      insights.stop();
      coverage.stop();
      trends.stop();
      activity.stop();
      poll.stop();
      window.clearInterval(tick);
      document.removeEventListener("kw:conn", paint);
    } };
  };
})();
