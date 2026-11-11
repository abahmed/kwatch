/* kwatch dashboard: the kwatch page (/kwatch): is kwatch itself
   working, can it see the whole cluster, and do its alerts get out.
   From /api/status (coverage totals, sends per channel over 24 hours,
   the state file and kwatch's own resources in its kwatch part; kwatch's
   own view of itself, its channels, version) and /api/coverage (the gaps
   and the rules to copy, ovcoverage.js). */
(function () {
  "use strict";
  var KW = window.KW;
  var DAY = 24 * 3600 * 1000;
  var HINT = "Check the provider settings in the installer: \"Edit notification providers\".";
  var H = KW.TERMS.health;
  // Memory, CPU and disk turn amber at 70% of their limit, red at 90%.
  var WARN = 0.7, BAD = 0.9;

  /* facts: label and value rows; a row's third item is a class for its
     value ("mono", "warn"). */
  function facts(rows) {
    var dl = KW.el("dl", "facts kp-facts");
    rows.forEach(function (r) {
      if (!r) return;
      dl.appendChild(KW.el("dt", "", r[0]));
      var dd = dl.appendChild(KW.el("dd", r[2] || ""));
      if (r[1] && r[1].nodeType) dd.appendChild(r[1]); else dd.textContent = r[1];
    });
    return dl;
  }

  function failingNow(c) {
    if (!KW.valid(c.lastError)) return false;
    return !KW.valid(c.lastSuccess) || Date.parse(c.lastError) > Date.parse(c.lastSuccess);
  }

  /* usage: how full a resource is: "bad" from 90% of its limit, "warn"
     from 70%, else "". */
  function usage(used, limit) {
    if (!(limit > 0)) return "";
    var p = used / limit;
    return p >= BAD ? "bad" : p >= WARN ? "warn" : "";
  }

  function memoryLevel(k) {
    var m = k && k.resources && k.resources.memory;
    return m ? usage(m.used, m.limit) : "";
  }

  /* problems: what is wrong with kwatch, in plain words, and how bad
     (cls "warn" or "bad"); the first thing wrong gives the headline. */
  function problems(s, k, issues) {
    var out = [], mem = memoryLevel(k), m = mem && k.resources.memory;
    var chans = (k && k.delivery) || [];
    if (chans.some(failingNow)) {
      out.push({ cls: "bad", head: "kwatch is running, but some alerts aren't getting out" });
    }
    if (mem === "bad") {
      out.push({ cls: "bad", head: "kwatch is almost out of memory",
        text: "It uses " + Math.round(m.used / m.limit * 100) + "% of its memory limit and may be restarted." });
    }
    var contact = s && s.self && s.self.lastApiContact;
    if (KW.valid(contact) && KW.now() - Date.parse(contact) > 5 * 60000) {
      out.push({ cls: "warn", head: "kwatch hasn't heard from the Kubernetes API for a while",
        text: "Last contact " + KW.rel(contact) + "." });
    }
    // The parts that do not work have their own notice above: counted
    // here, not said twice (they end the list, KW.selfIssues).
    var own = issues.length - KW.blindList(k).length;
    issues.forEach(function (t, i) { out.push({ cls: "warn", text: i < own ? KW.cap(t) + "." : "" }); });
    if (mem === "warn") {
      out.push({ cls: "warn", head: "kwatch is using most of its memory",
        text: "Memory is at " + Math.round(m.used / m.limit * 100) + "% of its limit." });
    }
    return out;
  }

  /* summary: one sentence about kwatch itself, then why when something
     is wrong, or the facts that show it works. While kwatch is still
     loading it says so, neither "working" nor "broken". */
  function summary(box, s, k, issues, loading) {
    box.textContent = "";
    var list = problems(s, k, issues), cls = "ok", text = H.ok, why = [];
    var self = (s && s.self) || {};
    if (KW.down()) {
      cls = "bad"; text = "Can't reach kwatch right now";
    } else if (loading) {
      cls = "wait"; text = "kwatch is starting up";
    } else if (list.length) {
      cls = list[0].cls;
      text = list[0].head || "kwatch is running, but " +
        KW.plural(list.length, "thing needs", "things need") + " a look";
      why = list.map(function (x) { return x.text; }).filter(Boolean);
    } else {
      var sent = ((k && k.delivery) || []).reduce(function (n, d) { return n + (d.sent || 0); }, 0);
      if (sent) why.push(KW.plural(sent, "alert", "alerts") + " sent in the last 24 hours.");
    }
    box.className = "ov-tile " + cls;
    var mark = box.appendChild(KW.el("span", "ov-mark"));
    mark.setAttribute("aria-hidden", "true");
    if (cls === "ok") mark.appendChild(KW.icon("check"));
    if (cls === "bad") mark.appendChild(KW.icon("x"));
    box.appendChild(KW.el("h2", "ov-say", text));
    if (why.length) box.appendChild(KW.el("p", "kp-why", why.join(" ")));
    if (cls === "wait") KW.startupLine(box, s);
    var np = KW.nonProdLine(k);
    if (np) box.appendChild(np);
  }

  /* blindNotices: one notice per optional part of kwatch that does not
     work (the API's kwatch.blindSpots), so a half-blind kwatch says so
     here and not only on /health; and one for issues waiting for their
     namespace's labels (KW.blindList). */
  function blindNotices(notes, k) {
    KW.blindList(k).forEach(function (b) {
      var w = KW.blindWords(b);
      var n = notes.appendChild(KW.el("div", "notice ov-note n-degraded"));
      n.dataset.blind = b.part;
      n.appendChild(KW.el("b", "", w.head));
      n.appendChild(KW.el("p", "", w.text));
      notes.hidden = false;
    });
  }

  function itself(s, k) {
    var c = KW.viz.card("kwatch itself");
    var self = s && s.self;
    if (!self) {
      c.appendChild(KW.viz.empty("kwatch hasn't reported on itself yet."));
      return c;
    }
    var stop = self.lastStop, last = null;
    if (stop && KW.valid(stop.at)) {
      last = KW.el("span");
      last.appendChild(KW.timeEl(stop.at));
      last.appendChild(document.createTextNode(" · " + (KW.STOPPED[stop.reason] || KW.STOPPED.unknown)));
    }
    c.appendChild(facts([
      KW.valid(self.startedAt) ? ["Running since", KW.timeEl(self.startedAt)] : null,
      last ? ["Last restart", last] : null,
      KW.valid(self.lastApiContact) ? [H.api, KW.timeEl(self.lastApiContact, "", true)] : null,
      // A queue of zero is the normal case: say so only when it isn't.
      self.outboxPending > 0 ? [H.waiting, KW.num(self.outboxPending), "warn"] : null
    ]));
    var now = KW.now(), list = (self.restarts || []).filter(function (r) {
      return KW.valid(r.at) && now - Date.parse(r.at) <= DAY;
    });
    var n = k && typeof k.restarts24h === "number" ? k.restarts24h : list.length;
    c.appendChild(KW.el("h3", "kp-sub", n ? KW.plural(n, "restart", "restarts") + " in the last 24 hours" :
      "No restarts in the last 24 hours"));
    c.appendChild(KW.viz.dots(list.map(function (r) {
      var why = KW.STOPPED[r.reason] || KW.STOPPED.unknown;
      return { at: r.at, text: KW.when(r.at) + " · " + why,
        cls: /oom_killed|internal_failure|eviction/.test(r.reason) ? "bad" : "" };
    }), now - DAY, now, "Restarts in the last 24 hours"));
    return c;
  }

  function stateFile(k) {
    var f = k && k.stateFile;
    if (!f) return null;
    var c = KW.viz.card("Saved history", "What kwatch keeps on disk: issues, history and unsent alerts.");
    if (f.capKnown && f.capBytes > 0) {
      var words = KW.size(f.bytes) + " of " + KW.size(f.capBytes) + " before kwatch trims it";
      var row = c.appendChild(KW.el("div", "kp-gauge"));
      row.appendChild(KW.viz.gauge(f.bytes, f.capBytes, "Saved history: " + words, WARN));
      row.appendChild(KW.el("p", "kp-gtext", words));
    } else {
      c.appendChild(KW.el("p", "kp-big", KW.size(f.bytes)));
    }
    return c;
  }

  /* cores: a CPU amount in plain digits ("0.04", "1.5"). */
  function cores(n) { return String(Number(n.toFixed(n < 1 ? 2 : 1))); }

  var LEVEL = { warn: "Getting high", bad: "Almost full" };

  /* resPart: one resource: a gauge against its limit, or the use alone
     when no limit is set. Amber from 70% of the limit, red from 90%,
     and the words say so too. */
  function resPart(box, name, used, limit, words, extra) {
    var part = box.appendChild(KW.el("div", "kp-res-part"));
    var lvl = usage(used, limit);
    if (limit > 0) part.appendChild(KW.viz.gauge(used, limit, name + ": " + words, WARN));
    var h = part.appendChild(KW.el("h3", "kp-res-h", name));
    if (lvl) h.appendChild(KW.chip(lvl === "bad" ? "c-page" : "c-notify", LEVEL[lvl]));
    part.appendChild(KW.el("p", "kp-res-t", words));
    if (extra) part.appendChild(KW.el("p", "kp-res-x", extra));
  }

  function pctOf(used, limit, of) { return Math.round(used / limit * 100) + "% of " + (of || "its limit"); }

  /* resources: kwatch's own memory, CPU and disk against its
     container's limits (/api/status resources); null when unknown. */
  function resources(k) {
    var r = k && k.resources;
    if (!r) return null;
    var c = KW.viz.card("Resources", "How much of its limits kwatch uses.");
    var box = c.appendChild(KW.el("div", "kp-res"));
    var m = r.memory, cpu = r.cpu, d = r.disk, f = r.stateFile;
    if (m) {
      resPart(box, "Memory", m.used, m.limit, m.limit > 0 ?
        pctOf(m.used, m.limit) + ": " + KW.size(m.used) + " of " + KW.size(m.limit) :
        KW.size(m.used) + " · no limit set",
        f && f.bytes > 0 ? "Includes its saved history (" + KW.size(f.bytes) + ")" : "");
    }
    if (cpu) {
      resPart(box, "CPU", cpu.cores, cpu.limitCores, cpu.limitCores > 0 ?
        pctOf(cpu.cores, cpu.limitCores) + ": " + cores(cpu.cores) + " of " +
        cores(cpu.limitCores) + " cores" : cores(cpu.cores) + " cores · no limit set");
    }
    if (d && d.total > 0) {
      var cap = f && f.capKnown && f.capBytes > 0 ? "; kwatch trims it at " + KW.size(f.capBytes) : "";
      resPart(box, "Disk", d.used, d.total, pctOf(d.used, d.total, "the volume") + ": " +
        KW.size(d.used) + " of " + KW.size(d.total), f ? "Saved history " + KW.size(f.bytes) + cap : "");
    }
    return box.firstChild ? c : null;
  }

  function coverage(k, cov, covErr) {
    var c = KW.viz.card("What kwatch is watching");
    var t = k && k.coverage;
    if (t) {
      var line = "Watching " + KW.num(t.watched) + " of " + KW.num(t.total) + " " + H.kinds;
      c.appendChild(KW.el("p", "kp-mline", line));
      c.appendChild(KW.viz.meter(t.watched, t.total, line, t.unreadable ? "warn" : "ok"));
      c.appendChild(KW.el("p", "pcard-note kp-note", H.kindsNote));
      // Only what is wrong: three zeros say nothing.
      var wrong = [[t.unreadable, "can't be read", "bad"], [t.unsynced, "not read yet", "warn"],
        [t.notWatched, "not watched", ""]].filter(function (x) { return x[0] > 0; });
      var stats = wrong.length ? c.appendChild(KW.el("ul", "kp-stats")) : null;
      wrong.forEach(function (x) {
        var li = stats.appendChild(KW.el("li", x[2]));
        li.appendChild(KW.el("b", "", KW.num(x[0])));
        li.appendChild(document.createTextNode(" " + x[1]));
      });
      c.appendChild(KW.el("p", "kp-ns", t.allNamespaces && !t.excludedNamespaces ?
        "All namespaces watched (" + KW.num(t.namespaces) + ")." :
        KW.plural(t.namespaces, "namespace", "namespaces") + " watched" +
        (t.excludedNamespaces ? ", " + KW.num(t.excludedNamespaces) + " left out" : "") + "."));
    }
    if (!t && !cov) {
      c.appendChild(KW.el("p", "kp-mline", covErr ?
        "kwatch couldn't say what it watches right now. This page tries again by itself." :
        "kwatch hasn't finished reading the cluster yet."));
    }
    if (cov) {
      var fold = c.appendChild(KW.el("details", "fold kp-fold"));
      fold.id = "cov";
      fold.appendChild(KW.el("summary", "", KW.coverageDenied(cov) ? "What it can't read, and the permission to add" : "Details"));
      var body = fold.appendChild(KW.el("div", "fold-body"));
      KW.coverageDraw(body, cov);
      return { card: c, fold: fold };
    }
    return { card: c, fold: null };
  }

  /* channel: one alert channel: its state in words, what it sent in
     the last 24 hours, and a bar of sent against failed only when
     something failed or waits to be retried. */
  function channel(d, h) {
    var li = KW.el("li", "kp-ch");
    var top = li.appendChild(KW.el("div", "kp-chtop"));
    top.appendChild(KW.el("span", "ch-name", d.name || d.provider));
    var bad = failingNow(d), state = KW.el("span", "ch-state " + (bad ? "bad" : KW.valid(d.lastSuccess) ? "ok" : "none"));
    state.appendChild(KW.icon(bad ? "x" : KW.valid(d.lastSuccess) ? "check" : "dash"));
    state.appendChild(KW.el("span", "", bad ? "Failing: last failed " + KW.rel(d.lastError) :
      KW.valid(d.lastSuccess) ? "Delivering: last sent " + KW.rel(d.lastSuccess) : "Nothing sent yet"));
    top.appendChild(state);
    var total = d.sent + d.failed + d.retrying;
    if (d.noCounts) {
      li.appendChild(KW.el("p", "kp-counts", "Counts for the last 24 hours are unavailable right now."));
      return detail(li, d, h, bad);
    }
    if (d.failed || d.retrying) {
      li.appendChild(KW.viz.stack([{ n: d.sent, cls: "s-ok", label: "Sent" }, { n: d.failed, cls: "s-page", label: "Failed" },
        { n: d.retrying, cls: "s-notify", label: "Retrying" }], (d.name || d.provider) + ", last 24 hours", total));
    }
    // Zeros are left out: "22 sent" is the whole story when all is well.
    var said = [];
    if (d.sent) said.push(KW.num(d.sent) + " sent");
    if (d.failed) said.push(KW.num(d.failed) + " failed");
    if (d.retrying) said.push(KW.num(d.retrying) + " waiting to retry");
    li.appendChild(KW.el("p", "kp-counts" + (d.failed ? " bad" : ""), total ? said.join(" · ") + " in the last 24 hours" :
      "Nothing to send in the last 24 hours."));
    return detail(li, d, h, bad);
  }

  /* detail: a channel's recent error, and what to do while it fails. */
  function detail(li, d, h, bad) {
    var msg = h && h.lastErrorMessage;
    if (bad || (msg && KW.valid(d.lastError) && KW.now() - Date.parse(d.lastError) < DAY)) {
      if (msg) li.appendChild(KW.el("p", "ch-msg", msg));
      if (bad) li.appendChild(KW.el("p", "ch-detail", (KW.valid(d.lastSuccess) ? "Last delivered " +
        KW.rel(d.lastSuccess) + ". " : "It has not delivered yet. ") + HINT));
    }
    return li;
  }

  function delivery(k, s) {
    var hrs = (k && k.deliveryWindowHours) || 24;
    var c = KW.viz.card("Where alerts go", "Alerts about issues, over the last " + hrs + " hours.");
    var health = (s && s.perChannelHealth) || [];
    var list = (k && k.delivery) || health.map(function (h) {
      return { name: h.name || h.channel, provider: h.type || h.channel, kind: "other", sent: 0,
        failed: 0, retrying: 0, lastSuccess: h.lastSuccess, lastError: h.lastError, noCounts: true };
    });
    if (!list.length) {
      c.appendChild(KW.el("p", "nochan", "No alert channels are set up yet."));
      c.appendChild(KW.el("p", "muted small", "Add one in the installer: choose \"Edit notification providers\"."));
      return c;
    }
    var ul = c.appendChild(KW.el("ul", "kp-chans"));
    list.forEach(function (d) {
      var h = health.filter(function (x) { return (x.type || x.channel) === d.provider || x.channel === d.provider; })[0];
      ul.appendChild(channel(d, h));
    });
    return c;
  }

  /* about: version, plan and what it watches: a slim card across the page. */
  function about(s) {
    var c = KW.viz.card("About this kwatch");
    c.classList.add("pwide", "kp-about");
    var ed = (s.edition || "").toLowerCase();
    var last = KW.valid(s.lastScan) ? KW.timeEl(s.lastScan, "", true) : "Not yet";
    var dl = KW.el("dl", "kp-facts-row");
    [["Version", s.version || "unknown", "mono"],
      ["Plan", ed === "pro" ? "Pro" : ed === "community" ? "Community" : s.edition || "unknown"],
      // scopeLine starts "Watching …": the label says it once.
      ["Last cluster check", last], ["Watching", KW.scopeLine(s).replace(/^Watching /, "")]].forEach(function (r) {
      var item = dl.appendChild(KW.el("div"));
      item.appendChild(KW.el("dt", "", r[0]));
      var dd = item.appendChild(KW.el("dd", r[2] || ""));
      if (r[1].nodeType) dd.appendChild(r[1]); else dd.textContent = r[1];
    });
    c.appendChild(dl);
    return c;
  }

  KW.kwatchView = function (main) {
    var tags = { c: "" }, k = null, cov = null, s = null, opened = false, covErr = false;
    var pg = KW.pageView(main, { title: "kwatch health", tab: "kwatch health · kwatch",
      sub: "Is kwatch itself working, can it see your cluster, and do alerts get out.", read: read, conn: draw });
    var notes = pg.body.appendChild(KW.el("div", "ov-notes"));
    var sum = pg.body.appendChild(KW.el("section"));
    var grid = pg.body.appendChild(KW.el("div", "pgrid"));
    pg.body.hidden = true;

    function draw() {
      if (!s && !k) return;
      var failed = (k && k.failedSendsLastHour) || (s && s.failedLastHour) || 0;
      var loading = KW.selfNotices(notes, s, failed, true, !!((cov && cov.syncedAt) ||
        (k && k.coverage && k.coverage.syncedAt)));
      if (s && s.self && s.self.podLogsForbidden) {
        var n = notes.appendChild(KW.el("div", "notice ov-note n-degraded"));
        n.appendChild(KW.el("b", "", "kwatch can't read pod logs"));
        n.appendChild(KW.el("p", "", "Issues show no log lines. Its role needs get on pods/log."));
        notes.hidden = false;
      }
      blindNotices(notes, k);
      summary(sum, s, k, KW.selfIssues(s, k), loading);
      var wasOpen = KW.$("#cov", grid), keep = wasOpen ? wasOpen.open : null;
      grid.textContent = "";
      // Half-width cards in pairs of about the same height: kwatch
      // itself beside its resources, alerts beside what it watches; an
      // odd one out takes the whole row. About goes across the bottom.
      var cv = coverage(k, cov, covErr);
      var halves = [itself(s, k), resources(k) || stateFile(k), delivery(k, s), cv.card]
        .filter(Boolean);
      halves.forEach(function (c) { grid.appendChild(c); });
      if (halves.length % 2) halves[halves.length - 1].classList.add("pwide");
      if (s) grid.appendChild(about(s));
      if (cv.fold) {
        cv.fold.open = keep !== null ? keep : KW.coverageDenied(cov) || location.hash === "#cov";
        if (!opened && location.hash === "#cov") cv.fold.scrollIntoView({ block: "start" });
        opened = true;
      }
    }

    function read() {
      return Promise.all([KW.api("/api/status"),
        KW.api("/api/coverage", { etag: tags.c })]).then(function (out) {
        if (out[0].status === 200 && out[0].data) { s = out[0].data; k = s.kwatch || k; }
        if (out[1].status === 200 && out[1].data) { tags.c = out[1].etag; cov = out[1].data; }
        covErr = out[1].status !== 200 && out[1].status !== 304;
        KW.selfDot(s, k);
        draw();
        return out[0];
      });
    }
    return { stop: pg.stop };
  };
})();
