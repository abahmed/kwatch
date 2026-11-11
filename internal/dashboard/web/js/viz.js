/* kwatch dashboard: small charts shared by the Overview and
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

  /* dots: points in time along a track from `from` to `to` (ms), each
     {at, text}: a dot with its words in the tooltip and for screen
     readers; ends say "24 h ago" and "now" unless
     ends names two other words. */
  V.dots = function (items, from, to, label, ends) {
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
    var names = ends;
    ends = box.appendChild(KW.el("div", "viz-tlends"));
    ends.setAttribute("aria-hidden", "true");
    ends.appendChild(KW.el("span", "", (names || [])[0] || "24 h ago"));
    ends.appendChild(KW.el("span", "", (names || [])[1] || "now"));
    return box;
  };

  /* gauge: a half circle filled to value of max, the percent inside;
     amber from warnAt (a fraction, 0.8 unless given), red from 0.9. */
  V.gauge = function (value, max, label, warnAt) {
    var p = pct(value, max) / 100;
    var s = svg("svg", { viewBox: "0 0 120 68", role: "img", "aria-label": label }, "viz-gauge");
    var arc = "M 10 60 A 50 50 0 0 1 110 60";
    s.appendChild(svg("path", { d: arc, pathLength: 100 }, "viz-gtrack"));
    var fill = svg("path", { d: arc, pathLength: 100, "stroke-dasharray": (p * 100) + " 100" },
      "viz-gfill" + (p >= 0.9 ? " bad" : p >= (warnAt || 0.8) ? " warn" : ""));
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

  /* card: a titled section of a page; note is a muted line under it
     and action (a link or a control) sits on the right of the title.
     Every card of a page has this one header (V.head). */
  V.card = function (title, note, wide, action) {
    var c = KW.el("section", "pcard" + (wide ? " pwide" : ""));
    V.head(c, title, note, action);
    return c;
  };

  /* head: a card's header: the title (h2), on its right an optional
     action, and under it an optional one-line note. cls adds classes
     to the h2 (the Overview's "ov-h2"). */
  V.head = function (box, title, note, action, cls) {
    var top = box.appendChild(KW.el("div", "pcard-top"));
    top.appendChild(KW.el("h2", "pcard-h" + (cls ? " " + cls : ""), title));
    if (action) top.appendChild(action).classList.add("pcard-act");
    if (note) box.appendChild(KW.el("p", "pcard-note", note));
  };

  /* seeAll: the "See all →" link a card's header carries. */
  V.seeAll = function (href, text) {
    var a = KW.el("a", "ulink", text || "See all →");
    a.href = href;
    a.dataset.link = "";
    return a;
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
