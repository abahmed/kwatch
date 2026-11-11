/* kwatch dashboard: the Overview, the landing page. It answers "is
   anything broken right now, and what do I look at first" first:
   notices only when someone must act (kwatch is starting, alert links
   are off, alerts failed to send), one status tile, the active issues
   by alert level as one line, up to eight issues to look at first (lookfit.js) and
   the releases and changes that caused issues (only when there are any).
   Below that, how the week went (ovinsights.js, trends.js). kwatch's
   own health is on the kwatch page (kwatchpage.js), the change feed on
   Activity (activity.js). Data: /api/overview, /api/status
   (its insights and trends parts). */
(function () {
  "use strict";
  var KW = window.KW;
  var V = KW.viz;
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
     that point at the kwatch page). It returns true while kwatch is
     still loading, so a page's verdict can say "starting" too: the
     verdict tile says it, with the progress (KW.startupLine), never a
     notice beside it saying the same. */
  KW.selfNotices = function (box, s, failed, onKwatch, readDone, hasData) {
    box.textContent = "";
    var self = (s && s.self) || {}, st = self.startup, links = self.dashboardLinks;
    var here = function (text) { return onKwatch ? null : link("/kwatch", text); };
    // readDone: the coverage says the first read of the cluster finished.
    // With issues to show there is data: nothing is "starting".
    var started = readDone || hasData || !!(st && st.done);
    var loading = !started && (!!st || !!(s && !KW.valid(s.lastScan) && !KW.down()));
    if (s && s.lastScanLate && !(st && !started)) {
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
    return loading;
  };

  /* KW.startupLine: under a "loading" verdict, how far the first read
     is: a meter and "12 of 40 kinds read · 3,200 objects". Nothing
     without the progress. */
  KW.startupLine = function (box, s) {
    var st = s && s.self && s.self.startup;
    if (!st || st.done || !(st.kinds > 0)) {
      box.appendChild(KW.el("p", "ov-sub ov-start", "This page updates by itself."));
      return;
    }
    var text = KW.num(st.synced) + " of " + KW.plural(st.kinds, "kind", "kinds") +
      " read · " + KW.plural(st.objects, "object", "objects");
    var m = box.appendChild(KW.viz.meter(st.synced, st.kinds, "Kinds read: " + text));
    m.classList.add("ov-start");
    box.appendChild(KW.el("p", "ov-sub ov-start", text + ". This page updates by itself."));
  };

  function totalOf(c) { return (c.urgent || 0) + (c.attention || 0) + (c.low || 0); }

  /* tile: the one line of the page: "1 urgent · 10 need attention · 29
     low", each part a link to that level's issues. Red only while
     something is urgent, amber while something needs attention, calm
     otherwise. */
  function tile(box, c, s, lastOk, hold) {
    box.textContent = "";
    var total = totalOf(c), cls, text = null, sub = "";
    var starting = s && ((s.self && s.self.startup && !s.self.startup.done) || !KW.valid(s.lastScan));
    if (KW.down() && lastOk) {
      cls = "stale";
      text = "Can't reach kwatch · last known at " + KW.hhmm(new Date(lastOk + KW.skew));
    } else if (!total && starting) {
      cls = "wait";
      text = "kwatch is loading what it knows";
    } else if (!total) {
      cls = "ok";
      text = "All clear · nothing is failing";
    } else {
      cls = c.urgent ? "bad" : c.attention ? "warn" : "ok";
      sub = KW.plural(total, "issue", "issues") + " happening now";
    }
    box.className = "ov-tile ov-status " + cls;
    var mark = box.appendChild(KW.el("span", "ov-mark"));
    mark.setAttribute("aria-hidden", "true");
    if (cls === "ok") mark.appendChild(KW.icon("check"));
    var say = box.appendChild(KW.el("div", "ov-says"));
    var h = say.appendChild(KW.el("h2", "ov-say"));
    h.id = "ov-say";
    if (text) h.textContent = text;
    else {
      LEVELS.filter(function (l) { return c[l[0]]; }).forEach(function (l, i) {
        if (i) h.appendChild(document.createTextNode(" · "));
        var a = link("/issues?status=active&severity=" + l[1],
          KW.num(c[l[0]]) + " " + KW.TERMS.lists.counts[l[1]], "ov-lvl " + l[2]);
        a.title = KW.SEVERITY[l[1]].title;
        h.appendChild(a);
      });
    }
    if (sub) say.appendChild(KW.el("p", "ov-sub", sub));
    if (cls === "wait") KW.startupLine(say, s);
    var held = KW.holdLine(hold);
    if (held) say.appendChild(held);
  }

  /* causeGist: what is wrong, in a few words, from the part of the
     title the short title leaves out ("tracking: the new version can't
     start; the previous one still serves"). "" when there is none. */
  function causeGist(title, i) {
    var full = String(title || ""), short = KW.shortTitle(full, i);
    var ns = i && i.namespace;
    if (ns) {
      var esc = ns.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      full = full.replace(new RegExp(" in " + esc + "( \\([^)]*\\))?(?=[ :.;,]|$)"), "");
    }
    if (!short || full.indexOf(short) !== 0 || short.slice(-1) === "…") return "";
    var rest = full.slice(short.length).replace(/^(\s*[:;.,—-]+\s*|\s+because\s+|\s+)/, "");
    rest = rest.replace(/[\s;:]+$/, "");
    if (rest.length > 120) {
      var cut = rest.lastIndexOf(" ", 119);
      rest = rest.slice(0, cut > 40 ? cut : 119) + "…";
    }
    return rest;
  }

  /* row: one issue to look at first: what, the gist of why, where, its
     level and how long ago it started. */
  function row(i) {
    var li = KW.el("li");
    var a = li.appendChild(link("/issues/" + encodeURIComponent(i.id), "", "ov-row"));
    a.dataset.rowlink = "";
    var mid = a.appendChild(KW.el("span", "ov-what"));
    var title = mid.appendChild(KW.el("span", "ov-title", KW.shortTitle(i.title || i.id, i)));
    title.title = i.title || i.id;
    var gist = causeGist(i.title, i);
    if (gist) mid.appendChild(KW.el("span", "ov-gist", gist));
    mid.appendChild(KW.wrapName(KW.whereText({ namespace: i.namespace,
      kind: i.kind, workload: i.name }), "ov-where mono"));
    var side = a.appendChild(KW.el("span", "ov-side"));
    var chips = side.appendChild(KW.el("span", "ov-chips"));
    chips.appendChild(KW.severityChip(i.severity));
    // Every row here is active: only a state other than "Happening now"
    // earns a pill.
    if (i.state !== "open") chips.appendChild(KW.stateChip(i.state));
    if (KW.valid(i.since)) {
      var t = side.appendChild(KW.el("span", "ov-dtext", "started " +
        KW.lasted(i.since, new Date(KW.now()).toISOString()) + " ago"));
      t.title = "Started " + KW.full(i.since);
    }
    if (i.acked) side.appendChild(KW.chip("c-acked ov-ack", "Acknowledged", "Someone acknowledged it."));
    return li;
  }

  function look(box, d) {
    box.textContent = "";
    var list = d.lookFirst || [], total = totalOf(d.counts || {});
    box.hidden = !list.length;
    if (!list.length) return;
    V.head(box, "Look at these first", KW.TERMS.lists.order,
      V.seeAll("/issues?status=active", "See all " + KW.plural(total, "issue", "issues") + " →"), "ov-h2");
    var ul = box.appendChild(KW.el("ul", "ov-look"));
    list.forEach(function (i) { ul.appendChild(row(i)); });
    // Every active issue is here when the server sent fewer than its cap.
    ul.dataset.all = list.length < KW.LOOK_LIMIT ? "1" : "";
    box.appendChild(KW.el("p", "ov-end", "That's everything happening now."));
  }

  var WRONG = { failed: "The release failed", down: "It went down after the release",
    regressed: "It got worse after the release" };

  /* relIssue: the issue a release that went wrong opened, resolved or
     not (the server links it: release_issues.go), or null. */
  function relIssue(w) {
    if (w.issue && w.issue.id) return w.issue;
    return w.incident ? { id: w.incident } : null;
  }

  /* workloadIssues: the Issues list filtered to a release's workload,
     for a release that opened no issue. */
  function workloadIssues(w) {
    return "/issues?status=all&namespace=" + encodeURIComponent(w.namespace || "") +
      "&kind=" + encodeURIComponent(String(w.kind || "").toLowerCase()) +
      "&q=" + encodeURIComponent(w.name || "");
  }

  /* relLine: when a release rolled out and what became of its issue:
     every row links somewhere, to its issue or to its workload's
     issues. */
  function relLine(at, w, issue, mins) {
    at.appendChild(document.createTextNode("Rolled out at "));
    at.appendChild(KW.hm(w.at));
    if (issue) {
      at.appendChild(document.createTextNode(issue.state === "resolved" ?
        ". Its issue is resolved." : "."));
      return;
    }
    at.appendChild(document.createTextNode(". No issue on " + w.name +
      (mins ? " opened in the " + mins + " minutes after it. " : ". ")));
    at.appendChild(link(workloadIssues(w), "See its issues →"));
  }

  /* changes: "Bad releases (last 24 h) and changes behind active
     issues": the releases of the window that failed, went down or
     regressed, joined with the active issues that started right after
     a change (one line per issue); only when there is one. */
  function changes(box, d) {
    box.textContent = "";
    var rels = d.releases || {};
    var rel = (rels.bad && rels.wrong) || [];
    var after = (d.insights && d.insights.afterChange && d.insights.afterChange.issues) || [];
    var by = {}, list = [];
    rel.forEach(function (w) {
      var e = { w: w, r: relIssue(w) };
      if (e.r) by[e.r.id] = e;
      list.push(e);
    });
    after.forEach(function (i) {
      if (by[i.id]) by[i.id].i = i;
      else list.push({ i: i });
    });
    box.hidden = !list.length;
    if (box.hidden) return;
    var hours = rels.windowHours || 24;
    V.head(box, KW.TERMS.lists.changes(hours), "Releases of the last " + hours +
      " hours that went wrong, and changes made just before an issue that is still going on.",
      null, "ov-h2");
    var ul = box.appendChild(KW.el("ul", "ov-rel-list"));
    list.forEach(function (e) {
      var li = ul.appendChild(KW.el("li"));
      var top = li.appendChild(KW.el("div", "ov-rel-top"));
      var id = e.i ? e.i.id : e.r && e.r.id;
      var name = e.i ? KW.shortTitle(e.i.title || e.i.id, { namespace: e.i.namespace }) :
        e.w.name + " in " + e.w.namespace;
      top.appendChild(link(id ? "/issues/" + encodeURIComponent(id) : workloadIssues(e.w),
        name, "ulink ov-rel-name"));
      if (e.w) top.appendChild(KW.el("span", "ov-rel-v", WRONG[e.w.verdict] || e.w.verdict));
      var at = li.appendChild(KW.el("div", "ov-rel-at"));
      if (e.i) {
        at.appendChild(document.createTextNode(e.i.object + ": " + e.i.change + " at "));
        at.appendChild(KW.hm(e.i.changedAt));
        var gap = KW.lasted(e.i.changedAt, e.i.since);
        at.appendChild(document.createTextNode(gap ? ", " + gap + " before the issue started." : "."));
      } else {
        relLine(at, e.w, e.r, rels.matchMinutes);
      }
    });
  }

  KW.statusView = function (main) {
    var etag = "", zoneNow = "", data = null, status = null, kw = null;
    // Made before pageView, which reads (read() below) right away.
    var weekBox = KW.el("section", "ov-week");
    var week = KW.insightsPart(weekBox);
    var pg = KW.pageView(main, { title: "Overview", read: read, conn: draw });
    var notes = pg.body.appendChild(KW.el("div", "ov-notes"));
    var tileBox = pg.body.appendChild(KW.el("section"));
    tileBox.setAttribute("aria-labelledby", "ov-say");
    // Under the verdict, what to act on: the issues to look at first
    // and, beside them on a wide screen, the bad releases and changes.
    var pair = pg.body.appendChild(KW.el("div", "ov-pair"));
    var main1 = pair.appendChild(KW.el("div", "ov-main"));
    // The issues and, under them, the old problems left out are one
    // card, the same height as the releases card beside it.
    var lookCard = main1.appendChild(KW.el("div", "pcard ov-act"));
    var lookBox = lookCard.appendChild(KW.el("section", "ov-first"));
    // Only when kwatch left old problems out.
    var old = lookCard.appendChild(KW.el("section", "ov-olds"));
    old.hidden = true;
    var calm = main1.appendChild(KW.el("p", "ov-calm"));
    calm.appendChild(link("/activity", "What changed recently"));
    // Only when a release or change caused an issue.
    var rel = pair.appendChild(KW.el("section", "pcard ov-act ov-rel"));
    rel.hidden = true;
    pg.body.appendChild(weekBox);
    pg.body.hidden = true;
    // Fills the issues card up to the height of the releases card.
    var fitter = KW.lookFit(pair, lookCard, rel);

    function draw() {
      if (!data) return;
      var c = data.counts || {};
      var failed = typeof data.failedSendsLastHour === "number" ? data.failedSendsLastHour :
        (status && status.failedLastHour) || 0;
      KW.selfNotices(notes, status, failed, false, false, totalOf(c) > 0);
      tile(tileBox, c, status, pg.lastOk(), data.scalingHold);
      look(lookBox, data);
      KW.oldProblems(old, data.oldProblems);
      lookCard.hidden = lookBox.hidden && old.hidden;
      changes(rel, data);
      fitter.fit();
      calm.hidden = totalOf(c) > 0;
    }

    function read() {
      var q = V.zone();
      if (q !== zoneNow) etag = "";
      zoneNow = q;
      return Promise.all([KW.api("/api/overview?" + q, { etag: etag }),
        KW.api("/api/status")]).then(function (out) {
        var r = out[0];
        if (out[1].status === 200 && out[1].data) {
          status = out[1].data;
          kw = status.kwatch || kw;
        }
        KW.selfDot(status, kw);
        if (r.status === 200 && r.data) {
          etag = r.etag;
          data = r.data;
          week.set(data.insights, data.trends);
          var c = data.counts || {};
          KW.updateBadge({ page: c.urgent || 0, notify: c.attention || 0,
            digest: c.low || 0, silent: c.quiet || 0 });
        }
        draw();
        return r;
      });
    }
    return { stop: function () { fitter.stop(); pg.stop(); } };
  };
})();
