/* kwatch dashboard: the Insights page (/insights): how the week went
   and where the trouble is, from /api/insights and /api/trends. "This
   week" as a column chart with a table beside it (trends.js), the top
   causes, the namespaces with active issues as stacked bars, the nodes
   with the most issues, and how many active issues started right
   after a change. Each section says so in plain words when it has
   nothing to show. */
(function () {
  "use strict";
  var KW = window.KW;
  var V = KW.viz;
  var SEVS = [["urgent", "page", "s-page"], ["attention", "notify", "s-notify"], ["low", "digest", "s-low"]];

  function link(href, text, cls) {
    var a = KW.el("a", cls || "ulink", text);
    a.href = href;
    a.dataset.link = "";
    return a;
  }

  function todayLine(t) {
    if (!t || !(t.new || t.resolved || t.urgent)) return "Nothing new today.";
    return "Today: " + KW.num(t.new) + " new, " + KW.num(t.resolved) + " resolved, " +
      KW.num(t.urgent) + " still urgent.";
  }

  function causes(list) {
    var c = V.card("Top causes this week", "What the issues of the last 7 days failed with.");
    if (!list || !list.length) {
      c.appendChild(V.empty("No issues this week, so no causes to show."));
      return c;
    }
    var max = 0;
    list.forEach(function (x) { max = Math.max(max, x.count); });
    var ul = c.appendChild(KW.el("ul", "viz-rows"));
    list.forEach(function (x) {
      ul.appendChild(V.row({ name: x.label, href: "/issues?status=all&mode=" + encodeURIComponent(x.mode),
        value: KW.plural(x.count, "time", "times") + (x.active ? " · " + KW.num(x.active) + " active" : ""), max: max,
        parts: [{ n: x.active, cls: "s-notify", label: "Still active" },
          { n: x.count - x.active, cls: "s-low", label: "Resolved" }] }));
    });
    c.appendChild(V.legend([{ cls: "s-notify", label: "Still active" }, { cls: "s-low", label: "Resolved" }]));
    return c;
  }

  function namespaces(m) {
    var c = V.card("Namespaces with active issues", "Most urgent first. Each opens its issues.");
    var tiles = (m && m.tiles) || [];
    if (!tiles.length) {
      c.appendChild(V.empty("No namespace has an active issue."));
      return c;
    }
    var max = 0;
    tiles.forEach(function (t) { max = Math.max(max, t.count); });
    var ul = c.appendChild(KW.el("ul", "viz-rows"));
    tiles.forEach(function (t) {
      var href = "/issues?status=active&namespace=" + encodeURIComponent(t.namespace);
      var words = SEVS.filter(function (s) { return t[s[0]]; }).map(function (s) {
        return KW.num(t[s[0]]) + " " + KW.SEVERITY[s[1]].label.toLowerCase();
      });
      ul.appendChild(V.row({ name: t.namespace, href: href, mono: true, max: max,
        value: words.join(" · ") || KW.plural(t.count, "issue", "issues"),
        parts: SEVS.map(function (s) {
          return { n: t[s[0]] || 0, cls: s[2], label: KW.SEVERITY[s[1]].label };
        }) }));
    });
    c.appendChild(V.legend(SEVS.map(function (s) { return { cls: s[2], label: KW.SEVERITY[s[1]].label }; })));
    if (m.more) {
      c.appendChild(KW.el("p", "pcard-more")).appendChild(link("/issues?status=active",
        "+" + KW.plural(m.more, "more namespace", "more namespaces")));
    }
    return c;
  }

  function nodes(list) {
    var c = V.card("Nodes with the most issues", "Active issues on each node.");
    if (!list || !list.length) {
      c.appendChild(V.empty("No node has an active issue."));
      return c;
    }
    var max = 0;
    list.forEach(function (n) { max = Math.max(max, n.issues); });
    var ul = c.appendChild(KW.el("ul", "viz-rows"));
    list.forEach(function (n) {
      ul.appendChild(V.row({ name: n.name, mono: true, max: max,
        href: "/issues?status=active&node=" + encodeURIComponent(n.name),
        value: KW.plural(n.issues, "issue", "issues") + (n.urgent ? " · " + KW.num(n.urgent) + " urgent" : ""),
        parts: [{ n: n.urgent, cls: "s-page", label: "Urgent" },
          { n: n.issues - n.urgent, cls: "s-low", label: "Other" }] }));
    });
    return c;
  }

  function afterChange(a) {
    var c = V.card("Started right after a change", "A deploy or config change shortly before it began.");
    var total = (a && a.total) || 0, n = (a && a.count) || 0;
    if (!total) {
      c.appendChild(V.empty("No active issues right now."));
      return c;
    }
    var line = KW.num(n) + " of " + KW.plural(total, "active issue", "active issues");
    c.appendChild(KW.el("p", "kp-mline", line));
    c.appendChild(V.meter(n, total, line + " started right after a change", "warn"));
    if (!n) {
      c.appendChild(V.empty("None of them followed a change."));
      return c;
    }
    var ul = c.appendChild(KW.el("ul", "ip-list"));
    (a.issues || []).forEach(function (i) {
      var li = ul.appendChild(KW.el("li"));
      KW.clampTitle(li.appendChild(link("/issues/" + encodeURIComponent(i.id), i.title || i.id, "ip-link")));
      li.appendChild(KW.el("span", "ip-meta", i.object + " " + i.change + ", " +
        KW.lasted(i.changedAt, i.since) + " before"));
    });
    return c;
  }

  KW.insightsView = function (main) {
    var tags = { i: "", t: "" }, ins = null, tr = null, zoneNow = "";
    var pg = KW.pageView(main, { title: "Insights", read: read,
      sub: "How the last 7 days went, and where the trouble is." });
    var lead = pg.body.appendChild(KW.el("p", "ip-today"));
    var grid = pg.body.appendChild(KW.el("div", "pgrid"));
    pg.body.hidden = true;

    function draw() {
      if (!ins) return;
      lead.textContent = todayLine(ins.today);
      grid.textContent = "";
      grid.appendChild(KW.weekCard(ins.days || [], tr));
      [causes(ins.causes), namespaces(ins.namespaces), nodes(ins.nodes),
        afterChange(ins.afterChange)].forEach(function (c) { grid.appendChild(c); });
      if (tr && tr.enough) KW.trendCards(tr).forEach(function (c) { grid.appendChild(c); });
    }

    function read() {
      var z = V.zone();
      if (z !== zoneNow) tags = { i: "", t: "" };
      zoneNow = z;
      return Promise.all([KW.api("/api/insights?" + z, { etag: tags.i }),
        KW.api("/api/trends?" + z, { etag: tags.t })]).then(function (out) {
        var changed = false;
        if (out[0].status === 200 && out[0].data) { tags.i = out[0].etag; ins = out[0].data; changed = true; }
        if (out[1].status === 200 && out[1].data) { tags.t = out[1].etag; tr = out[1].data; changed = true; }
        if (changed) draw();
        return out[0];
      });
    }
    return { stop: pg.stop };
  };
})();
