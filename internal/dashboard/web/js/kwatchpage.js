/* kwatch dashboard: the kwatch page (/kwatch): is kwatch itself
   working, can it see the whole cluster, and do its alerts get out.
   From /api/kwatch (coverage totals, sends per channel over 24 hours,
   the state file), /api/status (kwatch's own view of itself, its
   channels, version) and /api/coverage (the gaps and the rules to
   copy, ovcoverage.js). */
(function () {
  "use strict";
  var KW = window.KW;
  var DAY = 24 * 3600 * 1000;
  var HINT = "Check the provider settings in the installer: \"Edit notification providers\".";
  var KINDS = { pager: "Pager", chat: "Chat", email: "Email", other: "" };

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

  /* summary: one sentence about kwatch itself; the notices above it
     say what needs a look. */
  function summary(box, s, k, issues) {
    box.textContent = "";
    var bad = KW.down() || issues.length > 0;
    var chans = (k && k.delivery) || [];
    var text = KW.down() ? "Can't reach kwatch right now" :
      chans.some(failingNow) ? "kwatch is running, but some alerts aren't getting out" :
      issues.length ? "kwatch is running · " + KW.plural(issues.length, "thing needs", "things need") + " a look" :
      "kwatch is working and your alerts are getting out";
    box.className = "ov-tile " + (bad ? "warn" : "ok");
    var mark = box.appendChild(KW.el("span", "ov-mark"));
    mark.setAttribute("aria-hidden", "true");
    if (!bad) mark.appendChild(KW.icon("check"));
    box.appendChild(KW.el("h2", "ov-say", text));
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
      KW.valid(self.lastApiContact) ? ["Last API contact", KW.timeEl(self.lastApiContact, "", true)] : null,
      typeof self.outboxPending === "number" ? ["Messages waiting", KW.num(self.outboxPending)] : null
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
    var c = KW.viz.card("State file", "What kwatch keeps on disk: issues, history and the outbox.");
    if (f.capKnown && f.capBytes > 0) {
      var words = KW.size(f.bytes) + " of " + KW.size(f.capBytes) + " compaction cap";
      var row = c.appendChild(KW.el("div", "kp-gauge"));
      row.appendChild(KW.viz.gauge(f.bytes, f.capBytes, "State file: " + words));
      row.appendChild(KW.el("p", "kp-gtext", words));
      c.appendChild(KW.el("p", "pcard-note", "kwatch compacts the file to stay under the cap."));
    } else {
      c.appendChild(KW.el("p", "kp-big", KW.size(f.bytes)));
      c.appendChild(KW.el("p", "pcard-note", "No compaction cap is known."));
    }
    return c;
  }

  /* cores: a CPU amount in plain digits ("0.04", "1.5"). */
  function cores(n) { return String(Number(n.toFixed(n < 1 ? 2 : 1))); }

  /* resPart: one resource: a gauge against its limit, or the use alone
     when no limit is set. */
  function resPart(box, name, used, limit, words, extra) {
    var part = box.appendChild(KW.el("div", "kp-res-part"));
    if (limit > 0) part.appendChild(KW.viz.gauge(used, limit, name + ": " + words));
    part.appendChild(KW.el("h3", "kp-res-h", name));
    part.appendChild(KW.el("p", "kp-res-t", words));
    if (extra) part.appendChild(KW.el("p", "kp-res-x", extra));
  }

  function pctOf(used, limit) { return " (" + Math.round(used / limit * 100) + "%)"; }

  /* resources: kwatch's own memory, CPU and disk against its
     container's limits (/api/kwatch resources); null when unknown. */
  function resources(k) {
    var r = k && k.resources;
    if (!r) return null;
    var c = KW.viz.card("Resources");
    var box = c.appendChild(KW.el("div", "kp-res"));
    var m = r.memory, cpu = r.cpu, d = r.disk, f = r.stateFile;
    if (m) {
      resPart(box, "Memory", m.used, m.limit, m.limit > 0 ?
        KW.size(m.used) + " of " + KW.size(m.limit) + " limit" + pctOf(m.used, m.limit) :
        KW.size(m.used) + " · no limit set");
    }
    if (cpu) {
      resPart(box, "CPU", cpu.cores, cpu.limitCores, cpu.limitCores > 0 ?
        cores(cpu.cores) + " of " + cores(cpu.limitCores) + " cores" +
        pctOf(cpu.cores, cpu.limitCores) : cores(cpu.cores) + " cores · no limit set");
    }
    if (d && d.total > 0) {
      var cap = f && f.capKnown && f.capBytes > 0 ? " (cap " + KW.size(f.capBytes) + ")" : "";
      resPart(box, "Disk", d.used, d.total, KW.size(d.used) + " of " + KW.size(d.total) +
        " volume" + pctOf(d.used, d.total), f ? "State file " + KW.size(f.bytes) + cap : "");
    }
    return box.firstChild ? c : null;
  }

  function coverage(k, cov) {
    var c = KW.viz.card("What kwatch is watching", "", true);
    var t = k && k.coverage;
    if (t) {
      var line = KW.num(t.watched) + " of " + KW.plural(t.total, "kind", "kinds") + " of objects watched";
      c.appendChild(KW.el("p", "kp-mline", line));
      c.appendChild(KW.viz.meter(t.watched, t.total, line, t.unreadable ? "warn" : "ok"));
      var stats = c.appendChild(KW.el("ul", "kp-stats"));
      [[t.unreadable, "can't be read", t.unreadable ? "bad" : ""], [t.unsynced, "not read yet", t.unsynced ? "warn" : ""],
        [t.notWatched, "not watched", ""]].forEach(function (x) {
        var li = stats.appendChild(KW.el("li", x[2]));
        li.appendChild(KW.el("b", "", KW.num(x[0])));
        li.appendChild(document.createTextNode(" " + x[1]));
      });
      c.appendChild(KW.el("p", "kp-ns", t.allNamespaces && !t.excludedNamespaces ?
        "All namespaces watched (" + KW.num(t.namespaces) + ")." :
        KW.plural(t.namespaces, "namespace", "namespaces") + " watched" +
        (t.excludedNamespaces ? ", " + KW.num(t.excludedNamespaces) + " left out" : "") + "."));
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

  /* channel: one alert channel: its state, sends vs failures as a bar
     with counts, and the last error. */
  function channel(d, h) {
    var li = KW.el("li", "kp-ch");
    var top = li.appendChild(KW.el("div", "kp-chtop"));
    top.appendChild(KW.el("span", "ch-name", d.name || d.provider));
    if (KINDS[d.kind]) top.appendChild(KW.chip("c-low", KINDS[d.kind]));
    var bad = failingNow(d), state = KW.el("span", "ch-state " + (bad ? "bad" : KW.valid(d.lastSuccess) ? "ok" : "none"));
    state.appendChild(KW.icon(bad ? "x" : KW.valid(d.lastSuccess) ? "check" : "dash"));
    state.appendChild(KW.el("span", "", bad ? "last failed " + KW.rel(d.lastError) :
      KW.valid(d.lastSuccess) ? "delivering · last sent " + KW.rel(d.lastSuccess) : "nothing sent yet"));
    top.appendChild(state);
    var total = d.sent + d.failed + d.retrying;
    var parts = [{ n: d.sent, cls: "s-ok", label: "Sent" }, { n: d.failed, cls: "s-page", label: "Failed" },
      { n: d.retrying, cls: "s-notify", label: "Retrying" }];
    li.appendChild(KW.viz.stack(parts, (d.name || d.provider) + ", last 24 hours", total || 1));
    li.appendChild(KW.el("p", "kp-counts", total ? KW.num(d.sent) + " sent · " + KW.num(d.failed) + " failed" +
      (d.retrying ? " · " + KW.num(d.retrying) + " retrying" : "") : "No messages in the last 24 hours."));
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
    var c = KW.viz.card("Where alerts go", "Sent and failed in the last " + hrs +
      " hours. Approximate: counts incident messages only.", true);
    var health = (s && s.perChannelHealth) || [];
    var list = (k && k.delivery) || health.map(function (h) {
      return { name: h.name || h.channel, provider: h.type || h.channel, kind: "other", sent: 0,
        failed: 0, retrying: 0, lastSuccess: h.lastSuccess, lastError: h.lastError };
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

  function about(s) {
    var c = KW.viz.card("About this kwatch");
    var ed = (s.edition || "").toLowerCase();
    var last = KW.valid(s.lastScan) ? KW.timeEl(s.lastScan, "", true) : "Not yet";
    c.appendChild(facts([["Version", s.version || "unknown", "mono"],
      ["Plan", ed === "pro" ? "Pro" : ed === "community" ? "Community" : s.edition || "unknown"],
      ["Last check", last], ["Watching", KW.scopeLine(s)]]));
    return c;
  }

  /* misses: the cause self-check's likely misses (kwatch_status.go
     likelyMisses): issues that recovered after something other than the
     cause they named changed. Each links to its issue and, while the
     recording still holds it (48 hours), gives the export command. */
  function misses(k, s) {
    var list = (k && k.likelyMisses) || [];
    if (!list.length) return null;
    var c = KW.viz.card("Likely wrong causes (last 7 days)",
      "These recovered after something other than the cause they named changed.", true);
    var ul = c.appendChild(KW.el("ul", "ip-list"));
    var ns = (s && s.kwatchNamespace) || "kwatch";
    var dep = (s && s.kwatchDeployment) || "kwatch";
    list.forEach(function (m) {
      var li = ul.appendChild(KW.el("li"));
      var a = li.appendChild(KW.el("a", "ip-link", m.id));
      a.href = "/issues/" + encodeURIComponent(m.id);
      a.dataset.link = "";
      var said = "Named " + m.root + " (" + m.rule + ")";
      if (m.other) said += "; " + m.other + " changed" + (m.change ? " (" + m.change + ")" : "");
      li.appendChild(KW.el("span", "ip-meta", said));
      if (KW.valid(m.exportUntil) && KW.now() - Date.parse(m.exportUntil) < 48 * 3600 * 1000) {
        li.appendChild(KW.cmd("kubectl exec -n " + ns + " deploy/" + dep +
          " -- kwatch replay-export --since " + m.exportSince + " --until " + m.exportUntil +
          " --out /var/lib/kwatch/incident.jsonl"));
      }
    });
    return c;
  }

  KW.kwatchView = function (main) {
    var tags = { k: "", c: "" }, k = null, cov = null, s = null, opened = false;
    var pg = KW.pageView(main, { title: "kwatch", tab: "kwatch status · kwatch",
      sub: "Is kwatch itself working, can it see your cluster, and do alerts get out.", read: read, conn: draw });
    var notes = pg.body.appendChild(KW.el("div", "ov-notes"));
    var sum = pg.body.appendChild(KW.el("section"));
    var grid = pg.body.appendChild(KW.el("div", "pgrid"));
    pg.body.hidden = true;

    function draw() {
      if (!s && !k) return;
      var failed = (k && k.failedSendsLastHour) || (s && s.failedLastHour) || 0;
      KW.selfNotices(notes, s, failed, true, !!((cov && cov.syncedAt) ||
        (k && k.coverage && k.coverage.syncedAt)));
      if (s && s.self && s.self.podLogsForbidden) {
        var n = notes.appendChild(KW.el("div", "notice ov-note n-degraded"));
        n.appendChild(KW.el("b", "", "kwatch can't read pod logs"));
        n.appendChild(KW.el("p", "", "Issues show no log lines. Its role needs get on pods/log."));
        notes.hidden = false;
      }
      summary(sum, s, k, KW.selfIssues(s, k));
      var wasOpen = KW.$("#cov", grid), keep = wasOpen ? wasOpen.open : null;
      grid.textContent = "";
      // Half-width cards in pairs, so no row is left with a gap: kwatch
      // itself beside About, alerts beside Resources (alone, it
      // takes the whole row), then what kwatch watches across the page.
      var me = grid.appendChild(itself(s, k));
      if (s) grid.appendChild(about(s)); else me.classList.add("pwide");
      var dl = grid.appendChild(delivery(k, s));
      var f = resources(k) || stateFile(k);
      if (f) grid.appendChild(f); else dl.classList.add("pwide");
      var cv = coverage(k, cov);
      grid.appendChild(cv.card);
      if (cv.fold) {
        cv.fold.open = keep !== null ? keep : KW.coverageDenied(cov) || location.hash === "#cov";
        if (!opened && location.hash === "#cov") cv.fold.scrollIntoView({ block: "start" });
        opened = true;
      }
      var lm = misses(k, s);
      if (lm) grid.appendChild(lm);
    }

    function read() {
      return Promise.all([KW.api("/api/kwatch", { etag: tags.k }), KW.api("/api/status"),
        KW.api("/api/coverage", { etag: tags.c })]).then(function (out) {
        if (out[0].status === 200 && out[0].data) { tags.k = out[0].etag; k = out[0].data; }
        if (out[1].status === 200 && out[1].data) s = out[1].data;
        if (out[2].status === 200 && out[2].data) { tags.c = out[2].etag; cov = out[2].data; }
        KW.selfDot(s, k);
        draw();
        return out[0].status === 200 || out[0].status === 304 ? out[0] : out[1];
      });
    }
    return { stop: pg.stop };
  };
})();
