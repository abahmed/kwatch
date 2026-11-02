/* kwatch dashboard: memory (or CPU) as evidence. When an issue is about
   memory, or a container was killed for it or runs near its limit, the
   API sends each container's hourly peaks of the last 24 hours
   (usageHours); this draws them as columns with the limit as a dashed
   line, in SVG built with DOM calls (no inline markup), and says the
   same in words for screen readers. Other issues have no such part. */
(function () {
  "use strict";
  var KW = window.KW;
  var NS = "http://www.w3.org/2000/svg";
  var W = 480, H = 100, HOUR = 3600 * 1000;

  function svg(tag, attrs, cls) {
    var n = document.createElementNS(NS, tag);
    Object.keys(attrs).forEach(function (k) { n.setAttribute(k, String(attrs[k])); });
    if (cls) n.setAttribute("class", cls);
    return n;
  }

  function fmt(h) { return h.metric === "cpu" ? KW.fmtMilli : KW.fmtBytes; }

  /* summary: the chart in one or two sentences. */
  function summary(h) {
    var f = fmt(h), top = h.peaks[0], low = h.peaks[0];
    h.peaks.forEach(function (p) {
      if (p.value > top.value) top = p;
      if (p.value < low.value) low = p;
    });
    var what = h.metric === "cpu" ? "CPU" : "Memory";
    var text = what + " peaked at " + f(top.value) + " around " + KW.when(top.start);
    if (h.limit) text += ", " + Math.round(top.value / h.limit * 100) + "% of its " + f(h.limit) + " limit";
    else text += "; it has no limit";
    return text + ". Lowest hour: " + f(low.value) + ".";
  }

  function chart(h) {
    var top = h.limit || 0;
    h.peaks.forEach(function (p) { top = Math.max(top, p.value); });
    top = top * 1.08 || 1;
    var end = Date.parse(h.peaks[h.peaks.length - 1].start) + HOUR;
    var slot = W / 24;
    var s = svg("svg", { viewBox: "0 0 " + W + " " + H, preserveAspectRatio: "none",
      focusable: "false", "aria-hidden": "true" }, "uc-svg");
    s.appendChild(svg("line", { x1: 0, x2: W, y1: H - 0.5, y2: H - 0.5,
      "vector-effect": "non-scaling-stroke" }, "uc-base"));
    h.peaks.forEach(function (p) {
      var i = 24 - Math.round((end - Date.parse(p.start)) / HOUR);
      if (i < 0 || i > 23) return;
      var y = H - p.value / top * H;
      var hot = h.limit && p.value >= 0.9 * h.limit;
      var r = s.appendChild(svg("rect", { x: i * slot + 1, y: y, width: slot - 2,
        height: Math.max(1, H - y) }, hot ? "uc-col hot" : "uc-col"));
      r.appendChild(svg("title", {})).textContent = KW.when(p.start) + ": " + fmt(h)(p.value);
    });
    if (h.limit) {
      var ly = H - h.limit / top * H;
      s.appendChild(svg("line", { x1: 0, x2: W, y1: ly, y2: ly,
        "vector-effect": "non-scaling-stroke" }, "uc-limit"));
    }
    return { svg: s, from: new Date(end - 24 * HOUR).toISOString() };
  }

  function item(h) {
    var box = KW.el("div", "uc");
    box.appendChild(KW.el("p", "use-name mono", h.pod + " / " + h.container));
    var fig = box.appendChild(KW.el("figure", "uc-fig"));
    var c = chart(h);
    var text = summary(h);
    fig.setAttribute("role", "img");
    fig.setAttribute("aria-label", text);
    fig.appendChild(c.svg);
    var axis = fig.appendChild(KW.el("figcaption", "uc-axis"));
    axis.appendChild(KW.el("span", "", KW.when(c.from)));
    if (h.limit) {
      var key = axis.appendChild(KW.el("span", "uc-key"));
      key.appendChild(KW.el("span", "uc-dash"));
      key.appendChild(document.createTextNode("limit " + fmt(h)(h.limit)));
    }
    axis.appendChild(KW.el("span", "", "now"));
    box.appendChild(KW.el("p", "uc-sum", text));
    return box;
  }

  /* KW.usageChart draws the chart part of "Containers". */
  KW.usageChart = function (root, inc) {
    var box = KW.$("[data-usage]", root);
    var list = (inc.usageHours || []).filter(function (h) { return (h.peaks || []).length; });
    box.textContent = "";
    if (list.length) draw(box, list, inc);
    KW.containersFold(root);
  };

  function draw(box, list, inc) {
    var cpu = list.every(function (h) { return h.metric === "cpu"; });
    var mem = list.every(function (h) { return h.metric !== "cpu"; });
    box.appendChild(KW.el("h3", "fold-h", (cpu ? "CPU" : mem ? "Memory" : "Memory and CPU") +
      ", last 24 hours"));
    list.forEach(function (h) { box.appendChild(item(h)); });
    var cut = KW.cutCount(inc, "usageHours");
    box.appendChild(KW.el("p", "ev-cap", "The highest reading of each hour, from the node. " +
      (cpu ? "Shown because the container is being slowed down for using its CPU limit." :
        "Shown because this issue is about memory: a container that reaches its limit is stopped.") +
      (cut ? " " + KW.plural(cut, "more container", "more containers") + " not shown." : "")));
  }
})();
