/* kwatch dashboard: the Overview's short insight cards, from
   /api/insights: today in one line, the quiet-cluster line, urgent
   issues nobody acknowledged (the one urgent list, newest first),
   issues that started right after a change, alerts that didn't get
   out, the nodes with the most issues, the namespace map, and the top
   causes in "This week". Each card is left out when it has nothing to
   say. */
(function () {
  "use strict";
  var KW = window.KW;
  var SEV = { page: "page", notify: "notify", digest: "low", silent: "quiet" };

  function tz() {
    return KW.utc ? 0 : -new Date(KW.now()).getTimezoneOffset();
  }

  function nowISO() { return new Date(KW.now()).toISOString(); }

  var num = function (n) { return KW.num(n); };

  function link(href, text, cls) {
    var a = KW.el("a", cls || "in-link", text);
    a.href = href;
    a.dataset.link = "";
    return a;
  }

  function card(title, count) {
    var c = KW.el("section", "in-card");
    var h = KW.el("h3", "in-h", title);
    if (count) h.appendChild(KW.el("span", "in-n", " · " + count));
    c.appendChild(h);
    return c;
  }

  function list(c) {
    var ul = KW.el("ul", "in-list");
    c.appendChild(ul);
    return ul;
  }

  /* sevTag: the alert level in words, with its dot. */
  function sevTag(sev) {
    var s = KW.el("span", "in-sev");
    s.appendChild(KW.el("span", "dot " + (SEV[sev] || "low")));
    s.appendChild(document.createTextNode((KW.SEVERITY[sev] || {}).label || sev));
    return s;
  }

  function where(i) { return KW.whereText(i); }

  /* issue: a list entry for one issue, with what to say after it. */
  function issue(i, extra) {
    var li = KW.el("li", "in-item");
    li.appendChild(link("/issues/" + encodeURIComponent(i.id), i.title || i.id));
    var meta = KW.el("span", "in-meta");
    meta.appendChild(sevTag(i.severity));
    meta.appendChild(document.createTextNode(" · " + where(i)));
    li.appendChild(meta);
    if (extra) li.appendChild(KW.el("span", "in-meta", extra));
    return li;
  }

  function failingFor(i) { return "Failing for " + KW.lasted(i.since, nowISO()); }

  function todayLine(t) {
    if (!t || !(t.new || t.resolved || t.urgent)) return "";
    return "Today: " + num(t.new) + " new, " + num(t.resolved) + " resolved, " +
      num(t.urgent) + " still urgent.";
  }

  /* unacked: the one urgent list of the Overview, newest first, with
     "See all" and the same issues oldest first. */
  function unacked(u) {
    if (!u || !u.count) return null;
    var c = card("Urgent and not acknowledged", num(u.count) + ", newest first");
    c.classList.add("in-wide");
    var ul = list(c);
    u.issues.forEach(function (i) { ul.appendChild(issue(i, failingFor(i))); });
    var q = "/issues?status=active&severity=page&acked=false";
    var more = KW.el("p", "in-more");
    more.appendChild(link(q + "&sort=started", "See all " + num(u.count)));
    more.appendChild(document.createTextNode(" · "));
    more.appendChild(link(q + "&sort=started&dir=asc", "Oldest first"));
    c.appendChild(more);
    return c;
  }

  function afterChange(a) {
    if (!a || !a.count) return null;
    var c = card("Started right after a change", num(a.count));
    var ul = list(c);
    a.issues.forEach(function (i) {
      var gap = KW.lasted(i.changedAt, i.since);
      ul.appendChild(issue(i, "After: " + i.object + " " + i.change + " (" + gap + " before)"));
    });
    return c;
  }

  function undelivered(u) {
    if (!u || !u.count) return null;
    var c = card("Alerts that didn't get out", num(u.count) + " in the last 24 hours");
    var ul = list(c);
    u.attempts.forEach(function (a) {
      var li = KW.el("li", "in-item");
      var top = KW.el("span", "in-top");
      top.appendChild(KW.el("b", "", a.provider));
      top.appendChild(document.createTextNode(" · " +
        (a.result === "retrying" ? "still retrying" : "gave up") + " · "));
      top.appendChild(KW.timeEl(a.at));
      li.appendChild(top);
      li.appendChild(link("/issues/" + encodeURIComponent(a.id), a.title || a.id));
      if (a.error) li.appendChild(KW.el("span", "in-meta in-err", a.error));
      ul.appendChild(li);
    });
    return c;
  }

  function nodes(list0) {
    if (!list0 || !list0.length) return null;
    var c = card("Nodes with the most issues");
    var ul = list(c);
    list0.forEach(function (n) {
      var li = KW.el("li", "in-item in-row");
      li.appendChild(link("/issues?status=active&node=" + encodeURIComponent(n.name),
        n.name, "in-link mono"));
      li.appendChild(KW.el("span", "in-meta", KW.plural(n.issues, "issue", "issues") +
        (n.urgent ? ", " + num(n.urgent) + " urgent" : "")));
      ul.appendChild(li);
    });
    return c;
  }

  function causes(list0) {
    if (!list0 || !list0.length) return null;
    var c = card("Top causes this week");
    var ul = list(c);
    list0.forEach(function (x) {
      var li = KW.el("li", "in-item in-row");
      li.appendChild(link("/issues?status=all&mode=" + encodeURIComponent(x.mode), x.label));
      li.appendChild(KW.el("span", "in-meta", num(x.count) +
        (x.active ? " · " + num(x.active) + " still active" : "")));
      ul.appendChild(li);
    });
    return c;
  }

  function nsMap(box, m) {
    var tiles = (m && m.tiles) || [];
    box.hidden = !tiles.length;
    var ul = KW.$("[data-nsmap-list]", box);
    ul.textContent = "";
    tiles.forEach(function (t) {
      var li = KW.el("li");
      var a = link("/issues?status=active&namespace=" + encodeURIComponent(t.namespace),
        "", "ns-tile " + (SEV[t.severity] || "low"));
      var label = (KW.SEVERITY[t.severity] || {}).label || t.severity;
      a.appendChild(KW.el("span", "ns-name mono", t.namespace));
      a.appendChild(KW.el("span", "ns-meta", KW.plural(t.count, "issue", "issues") + " · " + label));
      a.title = t.namespace + ": " + KW.plural(t.count, "active issue", "active issues") +
        ", the worst " + label.toLowerCase();
      li.appendChild(a);
      ul.appendChild(li);
    });
    var more = KW.$("[data-nsmap-more]", box);
    more.hidden = !(m && m.more);
    more.textContent = m && m.more ? "+" + num(m.more) + " more namespaces" : "";
  }

  /* quietLine: what kwatch watches, in the same words as the Issues
     page (KW.scopeLine, from /api/status), with the counts of now. */
  function quietLine(box, q, status) {
    box.hidden = !q;
    if (!q) return;
    var ns = typeof q.namespaces === "number" ? q.namespaces : undefined;
    var text = status ? KW.scopeLine(status, ns) : "kwatch is watching your cluster";
    if (typeof q.objects === "number") text += " · " + KW.plural(q.objects, "object", "objects");
    var line = KW.$("[data-quiet-text]", box);
    line.textContent = text;
    if (KW.valid(q.lastScan)) {
      line.appendChild(document.createTextNode(" · last checked "));
      line.appendChild(KW.timeEl(q.lastScan, "", true));
    }
  }

  function draw(main, d, status) {
    var today = KW.$("[data-today]", main);
    today.textContent = todayLine(d.today);
    today.hidden = !today.textContent;
    quietLine(KW.$("[data-quiet]", main), d.quiet, status);
    var grid = KW.$("[data-ins-grid]", main);
    grid.textContent = "";
    [unacked(d.unacked), afterChange(d.afterChange),
      undelivered(d.undelivered), nodes(d.nodes)].forEach(function (c) {
      if (c) grid.appendChild(c);
    });
    KW.$("[data-ins]", main).hidden = !grid.children.length;
    nsMap(KW.$("[data-nsmap]", main), d.namespaces);
    var week = KW.$("[data-causes]", main), top = causes(d.causes);
    week.textContent = "";
    if (top) week.appendChild(top);
    KW.weekShown(main);
  }

  /* insightsView fills the Overview's insight boxes; the Overview
     calls load on its own refresh. A failed load keeps what is shown. */
  KW.insightsView = function (main) {
    var etag = "", stopped = false, zone = null, last = null, status = null;
    return {
      load: function () {
        var z = tz();
        if (z !== zone) etag = "";
        zone = z;
        return KW.api("/api/insights?tz=" + z, { etag: etag }).then(function (res) {
          if (stopped || res.status !== 200 || !res.data) return;
          etag = res.etag;
          last = res.data;
          draw(main, last, status);
        });
      },
      /* scope takes /api/status, for the quiet line's words. */
      scope: function (s) {
        status = s;
        if (last && !stopped) quietLine(KW.$("[data-quiet]", main), last.quiet, s);
      },
      stop: function () { stopped = true; }
    };
  };
})();
