/* kwatch dashboard: small charts shared by the Overview, Insights and
   kwatch pages. Every chart carries its numbers as text too and an
   aria-label that says what it shows, so color is never the only
   signal. Bars are plain boxes sized with el.style (CSSOM, allowed by
   the page's policy); the column chart and the gauge are inline SVG
   built with createElementNS. Colors come from CSS classes that use the
   tokens of app.css, so dark mode and high contrast follow. */
(function () {
  "use strict";
  var KW = window.KW;
  var SVG = "http://www.w3.org/2000/svg";
  var V = KW.viz = {};

  function svg(tag, attrs, cls) {
    var e = document.createElementNS(SVG, tag);
    Object.keys(attrs || {}).forEach(function (k) { e.setAttribute(k, attrs[k]); });
    if (cls) e.setAttribute("class", cls);
    return e;
  }

  function pct(n, max) { return max > 0 ? Math.max(0, Math.min(100, n / max * 100)) : 0; }

  /* stack: one horizontal bar of parts [{n, cls, label, href}], as wide
     as total/max of its track (max defaults to the total: a full bar).
     A part with href is a link. label says what the bar is, for screen
     readers; each part's tooltip has its count. */
  V.stack = function (parts, label, max) {
    var total = parts.reduce(function (s, p) { return s + (p.n || 0); }, 0);
    var track = KW.el("div", "viz-track");
    track.setAttribute("role", "img");
    var words = parts.filter(function (p) { return p.n; }).map(function (p) {
      return KW.num(p.n) + " " + p.label.toLowerCase();
    });
    track.setAttribute("aria-label", label + ": " + (words.join(", ") || "none"));
    var fill = track.appendChild(KW.el("div", "viz-fill"));
    fill.style.width = pct(total, max || total) + "%";
    parts.forEach(function (p) {
      if (!p.n) return;
      var seg = KW.el(p.href ? "a" : "span", "viz-seg " + (p.cls || ""));
      if (p.href) {
        seg.href = p.href;
        seg.dataset.link = "";
        seg.setAttribute("aria-hidden", "true");
        seg.tabIndex = -1;
      }
      seg.title = KW.num(p.n) + " " + p.label.toLowerCase();
      seg.style.flexGrow = String(p.n);
      fill.appendChild(seg);
    });
    return track;
  };

  /* meter: a progress bar of value out of max, with its words. */
  V.meter = function (value, max, label, cls) {
    var m = KW.el("div", "viz-meter " + (cls || ""));
    m.setAttribute("role", "progressbar");
    m.setAttribute("aria-valuemin", "0");
    m.setAttribute("aria-valuemax", String(max || 0));
    m.setAttribute("aria-valuenow", String(value || 0));
    m.setAttribute("aria-label", label);
    m.title = label;
    var f = m.appendChild(KW.el("span", "viz-mfill"));
    f.style.width = pct(value, max) + "%";
    return m;
  };

  /* row: a labelled horizontal bar: the name (a link when href), the
     count in words on the right, the bar under them. */
  V.row = function (o) {
    var li = KW.el("li", "viz-row");
    var top = li.appendChild(KW.el("div", "viz-rtop"));
    var name = KW.el(o.href ? "a" : "span", "viz-name" + (o.mono ? " mono" : ""), o.name);
    if (o.href) {
      name.href = o.href;
      name.dataset.link = "";
    }
    top.appendChild(name);
    top.appendChild(KW.el("span", "viz-val", o.value));
    li.appendChild(V.stack(o.parts, o.name, o.max));
    return li;
  };

  /* columns: the week as an SVG column chart, two columns a day:
     opened (with its paged part at the bottom) and resolved. days are
     [{label, opened, resolved, paged, title}]. */
  V.columns = function (days, label) {
    var W = 64, H = 150, top = 18, base = H - 22, colW = 20;
    var max = 1;
    days.forEach(function (d) { max = Math.max(max, d.opened, d.resolved); });
    var s = svg("svg", { viewBox: "0 0 " + days.length * W + " " + H, role: "img",
      "aria-label": label }, "viz-cols");
    s.appendChild(svg("line", { x1: 0, x2: days.length * W, y1: base + 0.5, y2: base + 0.5 }, "viz-axis"));
    function h(n) { return n ? Math.max(2, (base - top) * n / max) : 0; }
    function col(g, x, n, cls) {
      if (!n) return;
      g.appendChild(svg("rect", { x: x, y: base - h(n), width: colW, height: h(n), rx: 3 }, cls));
    }
    function num(g, x, n) {
      var t = svg("text", { x: x + colW / 2, y: base - h(n) - 4, "text-anchor": "middle" }, "viz-cnum");
      // A zero says so: an empty day reads as 0, not as missing.
      t.textContent = KW.num(n);
      g.appendChild(t);
    }
    days.forEach(function (d, i) {
      var g = svg("g", {}, "viz-day");
      var tt = svg("title");
      tt.textContent = d.title;
      g.appendChild(tt);
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
  };

  /* dots: points in time along a track from `from` to `to` (ms), each
     {at, text}: a dot with its words in the tooltip and for screen
     readers; ends say "24 h ago" and "now". */
  V.dots = function (items, from, to, label) {
    var box = KW.el("div", "viz-tl");
    var track = box.appendChild(KW.el("div", "viz-tltrack"));
    track.setAttribute("role", "list");
    track.setAttribute("aria-label", label);
    items.forEach(function (it) {
      var d = KW.el("span", "viz-dot " + (it.cls || ""));
      d.setAttribute("role", "listitem");
      d.tabIndex = 0;
      d.title = it.text;
      d.setAttribute("aria-label", it.text);
      d.style.left = pct(Date.parse(it.at) - from, to - from) + "%";
      track.appendChild(d);
    });
    var ends = box.appendChild(KW.el("div", "viz-tlends"));
    ends.setAttribute("aria-hidden", "true");
    ends.appendChild(KW.el("span", "", "24 h ago"));
    ends.appendChild(KW.el("span", "", "now"));
    return box;
  };

  /* gauge: a half circle filled to value of max, the percent inside. */
  V.gauge = function (value, max, label) {
    var p = pct(value, max) / 100;
    var s = svg("svg", { viewBox: "0 0 120 68", role: "img", "aria-label": label }, "viz-gauge");
    var arc = "M 10 60 A 50 50 0 0 1 110 60";
    s.appendChild(svg("path", { d: arc, pathLength: 100 }, "viz-gtrack"));
    var fill = svg("path", { d: arc, pathLength: 100, "stroke-dasharray": (p * 100) + " 100" },
      "viz-gfill" + (p >= 0.9 ? " bad" : p >= 0.75 ? " warn" : ""));
    if (p > 0) s.appendChild(fill);
    var t = svg("text", { x: 60, y: 58, "text-anchor": "middle" }, "viz-gtext");
    t.textContent = Math.round(p * 100) + "%";
    s.appendChild(t);
    return s;
  };

  /* legend: color keys with words, each optionally a link. */
  V.legend = function (keys) {
    var ul = KW.el("ul", "viz-legend");
    keys.forEach(function (k) {
      var li = ul.appendChild(KW.el("li"));
      var a = li.appendChild(KW.el(k.href ? "a" : "span", "viz-key"));
      if (k.href) {
        a.href = k.href;
        a.dataset.link = "";
      }
      a.appendChild(KW.el("span", "viz-sw " + k.cls));
      a.appendChild(KW.el("span", "", k.label));
      if (typeof k.n === "number") a.appendChild(KW.el("b", "viz-kn", KW.num(k.n)));
      if (k.title) a.title = k.title;
    });
    return ul;
  };

  /* card: a titled section of a page; note is a muted line under it. */
  V.card = function (title, note, wide) {
    var c = KW.el("section", "pcard" + (wide ? " pwide" : ""));
    c.appendChild(KW.el("h2", "pcard-h", title));
    if (note) c.appendChild(KW.el("p", "pcard-note", note));
    return c;
  };

  /* empty: a calm line for a section with nothing to show. */
  V.empty = function (text) { return KW.el("p", "pempty", text); };

  /* tz query of the zone the page shows times in (insights, trends). */
  V.zone = function () {
    var z = KW.utc ? 0 : -new Date(KW.now()).getTimezoneOffset(), name = "UTC";
    try { if (!KW.utc) name = Intl.DateTimeFormat().resolvedOptions().timeZone || ""; } catch (e) { name = ""; }
    return "tz=" + z + (name ? "&tzName=" + encodeURIComponent(name) : "");
  };
})();
