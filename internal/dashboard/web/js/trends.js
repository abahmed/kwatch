/* kwatch dashboard: parts of the Overview's "This week": the last
   seven days as a column chart (opened, with the part that paged
   someone, and resolved, per day, from the insights days of /api/overview) with the
   same numbers as a table, the issues that kept coming back and how
   long fixing took (from the trends of /api/overview). ovinsights.js reads the data
   and places them. */
(function () {
  "use strict";
  var KW = window.KW;
  var V = KW.viz;
  var WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

  function weekday(date, last) {
    if (last) return "Today";
    var p = date.split("-");
    return WEEKDAYS[new Date(Date.UTC(+p[0], +p[1] - 1, +p[2])).getUTCDay()];
  }

  /* KW.duration: seconds as "under a minute", "12 min", "3 h 10 min",
     "2 days". (KW.lasted takes times; the Unix epoch itself is not a
     valid one, so count from a second after it.) */
  KW.duration = function (sec) {
    if (!(sec >= 60)) return "under a minute";
    return KW.lasted(new Date(1000).toISOString(), new Date(1000 + sec * 1000).toISOString());
  };

  function table(rows, untold) {
    var d = KW.el("details", "ip-table");
    d.appendChild(KW.el("summary", "", "Show as a table"));
    var tb = d.appendChild(KW.el("table", "ip-t"));
    tb.appendChild(KW.el("caption", "sr-only", "Issues per day, last 7 days"));
    var hr = tb.appendChild(KW.el("thead")).appendChild(KW.el("tr"));
    ["Day", "Opened", "Alerted on-call", untold ? KW.UNTOLD.label : "", "Resolved"].forEach(function (h, i) {
      if (h) hr.appendChild(KW.el("th", i ? "num" : "", h)).scope = "col";
    });
    var body = tb.appendChild(KW.el("tbody"));
    rows.forEach(function (r) {
      var tr = body.appendChild(KW.el("tr"));
      tr.appendChild(KW.el("th", "", r.label + " " + r.date.slice(5))).scope = "row";
      (untold ? [r.opened, r.paged, r.untold, r.resolved] : [r.opened, r.paged, r.resolved])
        .forEach(function (n) { tr.appendChild(KW.el("td", "num", KW.num(n))); });
    });
    return d;
  }

  var SVG = "http://www.w3.org/2000/svg";
  function svg(tag, attrs, cls) {
    var e = document.createElementNS(SVG, tag);
    Object.keys(attrs || {}).forEach(function (k) { e.setAttribute(k, attrs[k]); });
    if (cls) e.setAttribute("class", cls);
    return e;
  }

  /* weekChart: the day columns drawn for the width the card gives them
     (the viewBox is that width, so the labels keep their size and the
     columns spread over the whole card, not a fixed 448 units).
     Per day, opened (its paged part at the bottom) beside resolved. */
  function weekChart(rows, label, width) {
    var H = 150, top = 18, base = H - 22;
    var W = width / rows.length;
    var colW = Math.max(12, Math.min(36, (W - 12) / 2 - 2));
    var max = 1;
    rows.forEach(function (d) { max = Math.max(max, d.opened, d.resolved); });
    var s = svg("svg", { viewBox: "0 0 " + width + " " + H, role: "img", "aria-label": label }, "viz-cols");
    s.appendChild(svg("line", { x1: 0, x2: width, y1: base + 0.5, y2: base + 0.5 }, "viz-axis"));
    function h(n) { return n ? Math.max(2, (base - top) * n / max) : 0; }
    function col(g, x, n, cls) {
      if (n) g.appendChild(svg("rect", { x: x, y: base - h(n), width: colW, height: h(n), rx: 3 }, cls));
    }
    function num(g, x, n) {
      // No "0" labels: the tooltip and the table still say 0.
      if (!n) return;
      var t = svg("text", { x: x + colW / 2, y: base - h(n) - 4, "text-anchor": "middle" }, "viz-cnum");
      t.textContent = KW.num(n);
      g.appendChild(t);
    }
    rows.forEach(function (d, i) {
      var g = svg("g", {}, "viz-day");
      g.appendChild(svg("title")).textContent = d.title;
      var x0 = i * W + (W - 2 * colW - 4) / 2, x1 = x0 + colW + 4;
      g.appendChild(svg("rect", { x: i * W, y: 0, width: W, height: H }, "viz-hit"));
      col(g, x0, d.opened, "viz-open");
      if (d.paged) {
        var ph = h(Math.min(d.paged, d.opened));
        g.appendChild(svg("rect", { x: x0, y: base - ph, width: colW, height: ph, rx: 3 }, "viz-paged"));
      }
      col(g, x1, d.resolved, "viz-res");
      num(g, x0, d.opened);
      num(g, x1, d.resolved);
      var day = svg("text", { x: i * W + W / 2, y: H - 6, "text-anchor": "middle" }, "viz-clabel");
      day.textContent = d.label;
      g.appendChild(day);
      s.appendChild(g);
    });
    return s;
  }

  /* KW.weekTotals: the week's openings, and how many of them settled
     on their own (no alert); a day without its own count takes the
     trends' for that date. */
  KW.weekTotals = function (days, t) {
    var byDate = {}, opened = 0, untold = 0;
    ((t && t.days) || []).forEach(function (d) { byDate[d.date] = d.untold || 0; });
    days.forEach(function (d) {
      opened += d.opened;
      untold += typeof d.untold === "number" ? d.untold : byDate[d.date] || 0;
    });
    return { opened: opened, untold: untold };
  };

  /* KW.weekCard: the week's chart, its key and its table; the totals
     are in the numbers above it (ovinsights.js). */
  KW.weekCard = function (days, t) {
    var c = V.card("Day by day", "", true);
    // Untold openings (settled on their own) are in opened, never in
    // paged; a day without its own count takes the trends' for that date.
    var opened = 0, resolved = 0, untold = 0, byDate = {};
    ((t && t.days) || []).forEach(function (d) { byDate[d.date] = d.untold || 0; });
    var rows = days.map(function (d, i) {
      var u = typeof d.untold === "number" ? d.untold : byDate[d.date] || 0;
      opened += d.opened; resolved += d.resolved; untold += u;
      var label = weekday(d.date, i === days.length - 1);
      return { date: d.date, label: label, opened: d.opened, resolved: d.resolved, paged: d.paged, untold: u,
        title: label + " (" + d.date + "): " + KW.num(d.opened) + " opened, " + KW.num(d.paged) +
          " alerted the on-call person, " + (u ? KW.num(u) + " settled before alerting, " : "") + KW.num(d.resolved) + " resolved" };
    });
    if (!opened && !resolved) {
      c.appendChild(V.empty("A quiet week: nothing opened or resolved."));
      return c;
    }
    var spoken = rows.map(function (r) {
      return r.label + ": " + KW.num(r.opened) + " opened, " + KW.num(r.resolved) + " resolved";
    });
    var wrap = c.appendChild(KW.el("div", "ip-chart"));
    var said = "Issues per day, last 7 days. " + spoken.join("; ");
    var drawn = 0;
    function draw() {
      var w = Math.round(wrap.clientWidth);
      if (!w || w === drawn) return;
      drawn = w;
      wrap.textContent = "";
      wrap.appendChild(weekChart(rows, said, w));
    }
    // Drawn once the card is on the page and again when its width changes.
    if (window.ResizeObserver) new ResizeObserver(draw).observe(wrap);
    else wrap.appendChild(weekChart(rows, said, 640));
    c.appendChild(V.legend([{ cls: "s-open", label: "Opened" }, { cls: "s-page", label: "Of those, alerted the on-call person" },
      { cls: "s-ok", label: "Resolved" }]));
    c.appendChild(table(rows, untold));
    return c;
  };

  function link(href, text) {
    var a = KW.el("a", "tr-link", text);
    a.href = href;
    a.dataset.link = "";
    return a;
  }

  /* KW.keptComingBack: the issues that opened more than once. */
  KW.keptComingBack = function (list) {
    var c = V.card("Kept coming back");
    var ol = KW.el("ol", "tr-list");
    list.forEach(function (n) {
      var li = KW.el("li", "tr-issue");
      KW.clampTitle(li.appendChild(link("/issues/" + encodeURIComponent(n.id), n.title || n.id)));
      var where = KW.el("span", "tr-meta");
      KW.where(n, where);
      li.appendChild(where);
      var words = "Opened " + (n.opened > 1 ? KW.num(n.opened) + " times" : "once") + " in the last 7 days";
      if (n.state === "flapping") words += ", keeps failing and recovering";
      li.appendChild(KW.el("span", "tr-meta", words + "."));
      ol.appendChild(li);
    });
    c.appendChild(ol);
    return c;
  };
})();
