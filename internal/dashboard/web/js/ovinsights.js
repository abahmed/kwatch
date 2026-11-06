/* kwatch dashboard: the Overview's "This week" part, below what is
   wrong now (status.js), from the insights and trends of /api/overview. Kept
   quiet on purpose: four numbers, the week chart (trends.js), the top
   causes beside where the trouble is (five rows each), and the rest
   (kept coming back, started after a change, nodes) folded under
   "More", each shown only when it has something to say. */
(function () {
  "use strict";
  var KW = window.KW;
  var V = KW.viz;
  var SEVS = [["urgent", "page", "s-page"], ["attention", "notify", "s-notify"], ["low", "digest", "s-low"]];
  var TOP = 5;

  function link(href, text, cls) {
    var a = KW.el("a", cls || "ulink", text);
    a.href = href;
    a.dataset.link = "";
    return a;
  }

  function seeAll(c, href, text) {
    c.appendChild(KW.el("p", "pcard-more")).appendChild(link(href, text));
  }

  /* stats: the week in four numbers. */
  function stats(ins, tr) {
    var opened = 0, resolved = 0, paged = 0;
    (ins.days || []).forEach(function (d) {
      opened += d.opened || 0; resolved += d.resolved || 0; paged += d.paged || 0;
    });
    var vs = "";
    if (tr && typeof tr.previousWeek === "number") {
      var diff = opened - tr.previousWeek;
      vs = !diff ? "same as last week" : KW.num(Math.abs(diff)) + (diff > 0 ? " more" : " fewer") + " than last week";
    }
    var r = tr && tr.resolve;
    var items = [["Opened", KW.num(opened), vs], ["Resolved", KW.num(resolved), ""],
      ["Paged the on-call person", KW.num(paged), ""],
      ["Time to resolve", r && r.count ? KW.duration(r.medianSeconds) : "–", r && r.count ? "median" : ""]];
    var dl = KW.el("dl", "ov-stats");
    items.forEach(function (it) {
      var d = dl.appendChild(KW.el("div", "ov-stat"));
      d.appendChild(KW.el("dt", "", it[0]));
      d.appendChild(KW.el("dd", "ov-stat-n", it[1]));
      if (it[2]) d.appendChild(KW.el("dd", "ov-stat-sub", it[2]));
    });
    return dl;
  }

  function causes(list) {
    if (!list || !list.length) return null;
    var c = V.card("Top causes");
    var max = 0;
    list.forEach(function (x) { max = Math.max(max, x.count); });
    var ul = c.appendChild(KW.el("ul", "viz-rows"));
    list.slice(0, TOP).forEach(function (x) {
      ul.appendChild(V.row({ name: x.label, href: "/issues?status=all&mode=" + encodeURIComponent(x.mode),
        value: KW.plural(x.count, "time", "times") + (x.active ? " · " + KW.num(x.active) + " active" : ""), max: max,
        parts: [{ n: x.active, cls: "s-notify", label: "Still active" },
          { n: x.count - x.active, cls: "s-low", label: "Resolved" }] }));
    });
    if (list.length > TOP) seeAll(c, "/issues?status=all", "See all issues of the week →");
    return c;
  }

  /* trouble: the namespaces with active issues, by alert level, with
     how many issues each opened this week (the trends of /api/overview). */
  function trouble(m, week) {
    var tiles = (m && m.tiles) || [];
    if (!tiles.length) return null;
    var opened = {};
    (week || []).forEach(function (n) { opened[n.namespace] = n.count; });
    var c = V.card("Where the trouble is");
    var max = 0;
    tiles.forEach(function (t) { max = Math.max(max, t.count); });
    var ul = c.appendChild(KW.el("ul", "viz-rows"));
    tiles.slice(0, TOP).forEach(function (t) {
      var words = SEVS.filter(function (s) { return t[s[0]]; }).map(function (s) {
        return KW.num(t[s[0]]) + " " + KW.SEVERITY[s[1]].label.toLowerCase();
      });
      if (opened[t.namespace]) words.push(KW.num(opened[t.namespace]) + " this week");
      ul.appendChild(V.row({ name: t.namespace, mono: true, max: max,
        href: "/issues?status=active&namespace=" + encodeURIComponent(t.namespace),
        value: words.join(" · ") || KW.plural(t.count, "issue", "issues"),
        parts: SEVS.map(function (s) {
          return { n: t[s[0]] || 0, cls: s[2], label: KW.SEVERITY[s[1]].label };
        }) }));
    });
    var more = (m.more || 0) + Math.max(0, tiles.length - TOP);
    if (more) seeAll(c, "/issues?status=active", "+" + KW.plural(more, "more namespace", "more namespaces"));
    return c;
  }

  function nodes(list) {
    if (!list || !list.length) return null;
    var c = V.card("Nodes with the most issues");
    var max = 0;
    list.forEach(function (n) { max = Math.max(max, n.issues); });
    var ul = c.appendChild(KW.el("ul", "viz-rows"));
    list.slice(0, TOP).forEach(function (n) {
      ul.appendChild(V.row({ name: n.name, mono: true, max: max,
        href: "/issues?status=active&node=" + encodeURIComponent(n.name),
        value: KW.plural(n.issues, "issue", "issues") + (n.urgent ? " · " + KW.num(n.urgent) + " urgent" : ""),
        parts: [{ n: n.urgent, cls: "s-page", label: "Urgent" },
          { n: n.issues - n.urgent, cls: "s-low", label: "Other" }] }));
    });
    return c;
  }

  function afterChange(a) {
    var n = (a && a.count) || 0, total = (a && a.total) || 0;
    if (!n || !total) return null;
    var c = V.card("Started right after a change");
    var line = KW.num(n) + " of " + KW.plural(total, "active issue", "active issues");
    c.appendChild(KW.el("p", "kp-mline", line));
    c.appendChild(V.meter(n, total, line + " started right after a change", "warn"));
    var ul = c.appendChild(KW.el("ul", "ip-list"));
    (a.issues || []).forEach(function (i) {
      var li = ul.appendChild(KW.el("li"));
      KW.clampTitle(li.appendChild(link("/issues/" + encodeURIComponent(i.id), i.title || i.id, "ip-link")));
      li.appendChild(KW.el("span", "ip-meta", i.object + " " + i.change + ", " +
        KW.lasted(i.changedAt, i.since) + " before"));
    });
    return c;
  }

  function add(grid, cards) {
    cards.forEach(function (c) { if (c) grid.appendChild(c); });
    return grid.childElementCount;
  }

  /* KW.insightsPart draws the part into box; set() redraws it from an /api/overview answer. */
  KW.insightsPart = function (box) {
    var ins = null, tr = null;
    box.setAttribute("aria-labelledby", "ov-week");
    var h = box.appendChild(KW.el("h2", "ov-h2", "This week"));
    h.id = "ov-week";
    var nums = box.appendChild(KW.el("div"));
    var week = box.appendChild(KW.el("div", "pgrid"));
    var main = box.appendChild(KW.el("div", "pgrid"));
    var fold = box.appendChild(KW.el("details", "ov-fold"));
    var sum = fold.appendChild(KW.el("summary"));
    var rest = fold.appendChild(KW.el("div", "pgrid"));
    // Hidden until the week is read: a failed read leaves no empty heading.
    box.hidden = true;

    function draw() {
      if (!ins) return;
      box.hidden = false;
      nums.textContent = "";
      nums.appendChild(stats(ins, tr));
      week.textContent = "";
      week.appendChild(KW.weekCard(ins.days || [], tr));
      main.textContent = "";
      main.hidden = !add(main, [causes(ins.causes), trouble(ins.namespaces, tr && tr.namespaces)]);
      rest.textContent = "";
      var cards = [tr && tr.noisiest && tr.noisiest.length ? KW.keptComingBack(tr.noisiest) : null,
        afterChange(ins.afterChange), nodes(ins.nodes)].filter(Boolean);
      add(rest, cards);
      fold.hidden = !cards.length;
      sum.textContent = "More: " + cards.map(function (c) {
        return KW.$(".pcard-h", c).textContent.toLowerCase();
      }).join(" · ");
    }

    /* set takes the insights and trends parts of an /api/overview
       answer and redraws. */
    function set(insights, trends) {
      if (!insights && !trends) return;
      if (insights) ins = insights;
      if (trends) tr = trends;
      draw();
    }
    return { set: set };
  };
})();
