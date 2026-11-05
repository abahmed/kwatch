/* kwatch dashboard: the Overview, the landing page. It answers "is
   anything broken right now, and what do I look at first" first:
   notices only when someone must act (kwatch is starting, alert links
   are off, alerts failed to send), one status tile, the active issues
   by alert level as one bar, and up to five issues to look at first.
   Below that, how the week went (ovinsights.js, trends.js). kwatch's
   own health is on the kwatch page (kwatchpage.js), the change feed on
   Activity (activity.js). Data: /api/overview, /api/status,
   /api/insights and /api/trends. */
(function () {
  "use strict";
  var KW = window.KW;
  var LEVELS = [["urgent", "page", "s-page"], ["attention", "notify", "s-notify"],
    ["low", "digest", "s-low"]];

  function link(href, text, cls) {
    var a = KW.el("a", cls || "ulink", text);
    a.href = href;
    a.dataset.link = "";
    return a;
  }

  /* notice: an actionable line at the top; more is a link. */
  function notice(box, cls, title, text, more, role) {
    var n = box.appendChild(KW.el("div", "notice ov-note " + cls));
    if (role) n.setAttribute("role", role);
    n.appendChild(KW.el("b", "", title));
    var p = KW.el("p", "", text || "");
    if (more) {
      if (text) p.appendChild(document.createTextNode(" "));
      p.appendChild(more);
    }
    if (text || more) n.appendChild(p);
    return n;
  }

  /* KW.selfNotices fills box with what about kwatch itself needs acting
     on now; the kwatch page uses it too (onKwatch leaves out the links
     that point at the kwatch page). */
  KW.selfNotices = function (box, s, failed, onKwatch, readDone) {
    box.textContent = "";
    var self = (s && s.self) || {}, st = self.startup, links = self.dashboardLinks;
    var here = function (text) { return onKwatch ? null : link("/kwatch", text); };
    // readDone: the coverage says the first read of the cluster finished.
    var started = readDone || !!(st && st.done);
    if (st && !started) {
      var n = notice(box, "", "kwatch is starting", "", null, "status");
      var text = KW.num(st.synced) + " of " + KW.plural(st.kinds, "kind", "kinds") +
        " read · " + KW.plural(st.objects, "object", "objects");
      n.appendChild(KW.viz.meter(st.synced, st.kinds, "Kinds read: " + text));
      n.appendChild(KW.el("p", "small", text + ". Problems show up once it has read the cluster."));
    } else if (s && !KW.valid(s.lastScan) && !KW.down() && !started) {
      notice(box, "", "kwatch is starting", "It hasn't finished its first check yet.", null, "status");
    } else if (s && s.lastScanLate) {
      notice(box, "n-degraded", "kwatch hasn't checked your cluster in a while",
        "Last check " + KW.rel(s.lastScan) + ".", here("See kwatch status"));
    }
    if (links && links.on === false) {
      notice(box, "n-degraded", "Alert links are off",
        links.text || "Alerts can't link to this page.", here("Details"));
    }
    if (failed > 0) {
      notice(box, "n-failing", KW.plural(failed, "alert", "alerts") +
        " failed to send in the last hour", "", here("See alert delivery"), "alert");
    }
    box.hidden = !box.firstChild;
  };

  function totalOf(c) { return (c.urgent || 0) + (c.attention || 0) + (c.low || 0); }

  /* tile: the one sentence of the page, with the freshness line. */
  function tile(box, c, s, updated, lastOk) {
    box.textContent = "";
    var total = totalOf(c), cls, text;
    var starting = s && ((s.self && s.self.startup && !s.self.startup.done) || !KW.valid(s.lastScan));
    if (KW.down() && lastOk) {
      cls = "stale";
      text = "Can't reach kwatch · last known at " + KW.hhmm(new Date(lastOk + KW.skew));
    } else if (!total && starting) {
      cls = "wait";
      text = "kwatch is starting · nothing found yet";
    } else if (!total) {
      cls = "ok";
      text = "All clear · nothing is failing";
    } else {
      cls = c.urgent ? "bad" : "warn";
      text = KW.plural(total, "issue", "issues") + " happening now · " +
        (c.urgent ? KW.num(c.urgent) + " urgent" : "none urgent");
    }
    box.className = "ov-tile " + cls;
    var mark = box.appendChild(KW.el("span", "ov-mark"));
    mark.setAttribute("aria-hidden", "true");
    if (cls === "ok") mark.appendChild(KW.icon("check"));
    var h = box.appendChild(KW.el("h2", "ov-say", text));
    h.id = "ov-say";
    box.appendChild(updated);
  }

  /* bar: the active issues by alert level; segments and keys link to
     the Issues table with that level. Quiet issues are a small count. */
  function bar(box, c) {
    box.textContent = "";
    var parts = LEVELS.map(function (l) {
      var sev = KW.SEVERITY[l[1]];
      return { n: c[l[0]] || 0, cls: l[2], label: sev.label, title: sev.title,
        href: "/issues?status=active&severity=" + l[1] };
    });
    var total = totalOf(c);
    box.appendChild(KW.el("h2", "sr-only", "Active issues by alert level"));
    box.appendChild(KW.viz.stack(parts, "Active issues by alert level", total || 1));
    var keys = parts.map(function (p) {
      return { cls: p.cls, label: p.label, n: p.n, href: p.href, title: p.title };
    });
    if (c.quiet) {
      keys.push({ cls: "s-quiet", label: "Quiet", n: c.quiet, title: KW.SEVERITY.silent.title,
        href: "/issues?status=active&severity=silent" });
    }
    box.appendChild(KW.viz.legend(keys));
  }

  /* row: one issue to look at first. */
  function row(i) {
    var li = KW.el("li");
    var a = li.appendChild(link("/issues/" + encodeURIComponent(i.id), "", "ov-row"));
    a.dataset.rowlink = "";
    var chips = a.appendChild(KW.el("span", "ov-chips"));
    chips.appendChild(KW.severityChip(i.severity));
    chips.appendChild(KW.stateChip(i.state));
    var mid = a.appendChild(KW.el("span", "ov-what"));
    KW.clampTitle(mid.appendChild(KW.el("span", "ov-title", i.title || i.id)));
    mid.appendChild(KW.el("span", "ov-where mono", KW.whereText({ namespace: i.namespace,
      kind: i.kind, workload: i.name })));
    var dur = a.appendChild(KW.el("span", "ov-dur"));
    var t = dur.appendChild(KW.el("span", "ov-dtext", KW.forLong(i.since)));
    if (KW.valid(i.since)) t.title = "Started " + KW.full(i.since);
    if (i.acked) dur.appendChild(KW.chip("c-acked ov-ack", "Acknowledged", "Someone acknowledged it."));
    return li;
  }

  function look(box, d) {
    box.textContent = "";
    var list = d.lookFirst || [], total = totalOf(d.counts || {});
    box.hidden = !list.length;
    if (!list.length) return;
    box.appendChild(KW.el("h2", "ov-h2", "Look at these first"));
    var ul = box.appendChild(KW.el("ul", "ov-look"));
    list.forEach(function (i) { ul.appendChild(row(i)); });
    var more = box.appendChild(KW.el("p", "ov-more"));
    more.appendChild(link("/issues?status=active", "See all " + KW.plural(total, "issue", "issues") + " →"));
  }

  /* releasesLine: "4 releases in the last 24 hours, 1 went wrong", a
     link to the Releases page, or nothing when there were none. */
  function releasesLine(box, res) {
    var d = res && res.status === 200 && res.data;
    box.textContent = "";
    box.hidden = !d || !d.total;
    if (box.hidden) return;
    var text = KW.plural(d.total, "release", "releases") + " in the last 24 hours" +
      (d.bad ? ", " + KW.num(d.bad) + " went wrong" : ", all fine so far");
    box.appendChild(link(d.bad ? "/releases?since=24h&verdict=bad" : "/releases?since=24h", text));
  }

  KW.statusView = function (main) {
    var etag = "", data = null, status = null, kw = null;
    // Made before pageView, which reads (read() below) right away.
    var weekBox = KW.el("section", "ov-week");
    var week = KW.insightsPart(weekBox);
    var pg = KW.pageView(main, { title: "Overview", read: read, conn: draw });
    var notes = pg.body.appendChild(KW.el("div", "ov-notes"));
    var tileBox = pg.body.appendChild(KW.el("section"));
    tileBox.setAttribute("aria-labelledby", "ov-say");
    var barBox = pg.body.appendChild(KW.el("section", "ov-sev"));
    var lookBox = pg.body.appendChild(KW.el("section", "ov-first"));
    var calm = pg.body.appendChild(KW.el("p", "ov-calm"));
    calm.appendChild(link("/activity", "What changed recently"));
    // One quiet line about today's releases (releases.js), only when
    // there were some.
    var rel = pg.body.appendChild(KW.el("p", "ov-rel"));
    rel.hidden = true;
    pg.body.appendChild(weekBox);
    pg.body.hidden = true;

    function draw() {
      if (!data) return;
      var c = data.counts || {};
      var failed = typeof data.failedSendsLastHour === "number" ? data.failedSendsLastHour :
        (status && status.failedLastHour) || 0;
      KW.selfNotices(notes, status, failed);
      tile(tileBox, c, status, pg.updated, pg.lastOk());
      bar(barBox, c);
      look(lookBox, data);
      calm.hidden = totalOf(c) > 0;
    }

    function read() {
      return Promise.all([KW.api("/api/overview", { etag: etag }), KW.api("/api/status"),
        kw ? null : KW.api("/api/kwatch"), week.read(),
        KW.api("/api/releases?since=24h")]).then(function (out) {
        var r = out[0];
        releasesLine(rel, out[4]);
        if (out[1].status === 200 && out[1].data) status = out[1].data;
        if (out[2] && out[2].status === 200) kw = out[2].data;
        KW.selfDot(status, kw);
        if (r.status === 200 && r.data) {
          etag = r.etag;
          data = r.data;
          var c = data.counts || {};
          KW.updateBadge({ page: c.urgent || 0, notify: c.attention || 0,
            digest: c.low || 0, silent: c.quiet || 0 });
        }
        draw();
        return r;
      });
    }
    return { stop: pg.stop };
  };
})();
