/* kwatch dashboard: the Overview's "This week" part, below what is
   wrong now (status.js), from the insights and trends of /api/overview.
   Kept quiet on purpose: up to four numbers (none of them a blank),
   all over the issues someone was alerted about, one plain sentence on
   the issues that settled on their own, the week
   chart (trends.js), the top causes beside where the trouble is (five
   rows each, each number saying what it counts) and the rest (kept
   coming back, nodes) folded under "More", each shown only when it has
   something to say. Issues that started right after a change are in the
   "Bad releases and changes" block above (status.js). */
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

  /* stats: the week in four numbers, each over the issues someone was
     alerted about: those that settled on their own are the side note
     below (settled). An older kwatch without the alerted counts gives
     the plain ones. */
  function stats(ins, tr) {
    var resolved = 0, paged = 0, L = KW.TERMS.lists;
    var w = KW.weekTotals(ins.days || [], tr), alerted = w.opened - w.untold;
    (ins.days || []).forEach(function (d) {
      resolved += typeof d.resolvedAlerted === "number" ? d.resolvedAlerted : d.resolved || 0;
      paged += d.paged || 0;
    });
    var vs = "", prev = tr && (typeof tr.previousWeekAlerted === "number" ?
      tr.previousWeekAlerted : tr.previousWeek);
    if (typeof prev === "number") {
      var diff = alerted - prev;
      vs = !diff ? "same as last week" : KW.num(Math.abs(diff)) + (diff > 0 ? " more" : " fewer") + " than last week";
    }
    var r = tr && (tr.resolveAlerted || tr.resolve);
    var items = [[L.alerted, KW.num(alerted), vs || L.alertedSub],
      [L.resolvedAlerted, KW.num(resolved), L.resolvedAlertedSub],
      [L.alertedOnCall, KW.num(paged), L.alertedOnCallSub],
      r && r.count ? [L.medianResolveAlerted, KW.duration(r.medianSeconds),
        "over " + KW.plural(r.count, "alerted issue", "alerted issues") + " resolved"] : null].filter(Boolean);
    var dl = KW.el("dl", "ov-stats");
    items.forEach(function (it) {
      var d = dl.appendChild(KW.el("div", "ov-stat"));
      d.appendChild(KW.el("dt", "", it[0]));
      d.appendChild(KW.el("dd", "ov-stat-n", it[1]));
      if (it[2]) d.appendChild(KW.el("dd", "ov-stat-sub", it[2]));
    });
    return dl;
  }

  /* settled: the week's noise in one plain sentence: most issues
     recovered before kwatch had to alert anyone. */
  function settled(ins, tr) {
    var w = KW.weekTotals(ins.days || [], tr);
    if (!w.untold || !w.opened) return null;
    var p = KW.el("p", "ov-settled");
    p.appendChild(KW.el("b", "", KW.num(w.untold)));
    p.appendChild(document.createTextNode(" of " + KW.num(w.opened) +
      " issues settled on their own before anyone was alerted."));
    return p;
  }

  function causes(list) {
    if (!list || !list.length) return null;
    var c = V.card("Top causes this week");
    var max = 0;
    list.forEach(function (x) { max = Math.max(max, x.count); });
    var ul = c.appendChild(KW.el("ul", "viz-rows"));
    list.slice(0, TOP).forEach(function (x) {
      ul.appendChild(V.row({ name: x.label, href: "/issues?status=all&mode=" + encodeURIComponent(x.mode),
        value: KW.plural(x.count, "issue", "issues") + " this week" +
          (x.active ? " · " + KW.num(x.active) + " still going on" : ""), max: max,
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
      var words = [KW.num(t.count) + " active now" + (t.urgent ? " (" + KW.num(t.urgent) + " urgent)" : "")];
      if (opened[t.namespace]) words.push(KW.plural(opened[t.namespace], "issue", "issues") + " this week");
      ul.appendChild(V.row({ name: t.namespace, mono: true, max: max,
        href: "/issues?status=active&namespace=" + encodeURIComponent(t.namespace),
        value: words.join(" · "),
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

  function add(grid, cards) {
    cards.forEach(function (c) { if (c) grid.appendChild(c); });
    return grid.childElementCount;
  }

  /* KW.insightsPart draws the part into box; set() redraws it from an /api/overview answer. */
  KW.insightsPart = function (box) {
    var ins = null, tr = null;
    box.setAttribute("aria-labelledby", "ov-week");
    var h = box.appendChild(KW.el("h2", "ov-h2", "This week (last 7 days)"));
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
      var said = settled(ins, tr);
      if (said) nums.appendChild(said);
      week.textContent = "";
      week.appendChild(KW.weekCard(ins.days || [], tr));
      main.textContent = "";
      main.hidden = !add(main, [causes(ins.causes), trouble(ins.namespaces, tr && tr.namespaces)]);
      rest.textContent = "";
      var cards = [tr && tr.noisiest && tr.noisiest.length ? KW.keptComingBack(tr.noisiest) : null,
        nodes(ins.nodes)].filter(Boolean);
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
