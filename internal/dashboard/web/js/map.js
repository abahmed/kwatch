/* kwatch dashboard: the picture on an issue's Impact tab ("How it
   connects"), from the detail's map: the objects the issue touches and
   those next to them, laid out left to right by maplayout.js and drawn
   as SVG. Routes, Services and workloads (pods folded in) are boxes
   colored by the worst open issue touching them; the issue's own object
   is outlined. A box with another issue opens it. The same objects and
   connections are a plain list below, which is all a narrow screen
   shows. */
(function () {
  "use strict";
  var KW = window.KW;
  var SVG = "http://www.w3.org/2000/svg";
  var LEVEL = { page: "Urgent", notify: "Needs attention", digest: "Low", ok: "No open issue" };
  var EDGE = { routes: "routes to", selects: "sends to", calls: "calls" };

  function svg(tag, attrs, cls) {
    var e = document.createElementNS(SVG, tag);
    Object.keys(attrs || {}).forEach(function (k) { e.setAttribute(k, attrs[k]); });
    if (cls) e.setAttribute("class", cls);
    return e;
  }

  function label(n) { return n.kind + " " + (n.namespace ? n.namespace + "/" : "") + n.name; }

  function short(text, max) { return text.length > max ? text.slice(0, max - 1) + "…" : text; }

  function issueHref(id) { return "/issues/" + encodeURIComponent(id); }

  function picture(canvas, data, selfIssue) {
    var nodes = data.nodes, edges = data.edges;
    var lay = KW.mapLayout(nodes, edges);
    var root = canvas.appendChild(svg("svg", { viewBox: "0 0 " + lay.width + " " + lay.height,
      width: lay.width, height: lay.height, role: "group",
      "aria-label": "How it connects: " + KW.plural(nodes.length, "object", "objects") }, "map-svg"));
    edges.forEach(function (e) {
      var a = lay.pos[e.from], b = lay.pos[e.to];
      if (!a || !b) return;
      var x1 = a.x + lay.w, y1 = a.y + lay.h / 2, x2 = b.x, y2 = b.y + lay.h / 2, mid = (x1 + x2) / 2;
      root.appendChild(svg("path", { d: "M" + x1 + " " + y1 + " C" + mid + " " + y1 + " " + mid +
        " " + y2 + " " + x2 + " " + y2, "aria-hidden": "true" }, "map-edge map-e-" + e.type));
    });
    nodes.forEach(function (n) {
      var p = lay.pos[n.id];
      var self = n.id === data.self;
      var link = n.issue && n.issue !== selfIssue;
      var g = root.appendChild(svg("g", { transform: "translate(" + p.x + " " + p.y + ")" },
        "map-node map-l-" + n.level + (n.external ? " map-ext" : "") + (self ? " map-self" : "")));
      g.appendChild(svg("title")).textContent = label(n) + " · " + LEVEL[n.level];
      g.appendChild(svg("rect", { width: lay.w, height: lay.h, rx: 6 }));
      g.appendChild(svg("text", { x: 10, y: 16 }, "map-kind")).textContent = n.kind +
        (n.ready ? " · " + n.ready + " ready" : "");
      g.appendChild(svg("text", { x: 10, y: 32 }, "map-name")).textContent = short(n.name, 22);
      if (!link) return;
      g.setAttribute("tabindex", "0");
      g.setAttribute("role", "link");
      g.setAttribute("aria-label", label(n) + ", " + LEVEL[n.level] + ": open its issue");
      var act = function () { KW.go(issueHref(n.issue)); };
      g.addEventListener("click", act);
      g.addEventListener("keydown", function (ev) {
        if (ev.key === "Enter") { ev.preventDefault(); act(); }
      });
    });
  }

  function list(box, data, selfIssue) {
    var byId = {};
    data.nodes.forEach(function (n) { byId[n.id] = n; });
    data.nodes.forEach(function (n) {
      var li = box.appendChild(KW.el("li"));
      var name = li.appendChild(KW.el(n.issue && n.issue !== selfIssue ? "a" : "span", "map-li",
        label(n) + (n.id === data.self ? " (this issue)" : "")));
      if (name.tagName === "A") {
        name.href = issueHref(n.issue);
        name.dataset.link = "";
      }
      li.appendChild(KW.el("span", "map-li-level map-l-" + n.level, LEVEL[n.level]));
      var out = data.edges.filter(function (e) { return e.from === n.id && byId[e.to]; });
      if (out.length) {
        li.appendChild(KW.el("span", "map-li-out", out.map(function (e) {
          return EDGE[e.type] + " " + label(byId[e.to]);
        }).join("; ")));
      }
    });
  }

  /* KW.issueMap fills the "How it connects" fold of an issue's page from
     inc.map, or keeps it hidden when kwatch knows no connection. */
  KW.issueMap = function (root, inc) {
    var fold = KW.$('[data-fold="map"]', root);
    if (!fold) return;
    var data = inc.map;
    var canvas = KW.$("[data-map-canvas]", fold), box = KW.$("[data-map-list]", fold);
    canvas.textContent = "";
    box.textContent = "";
    fold.hidden = !(data && data.nodes && data.nodes.length);
    if (fold.hidden) return;
    var failing = data.nodes.filter(function (n) { return n.level !== "ok"; }).length;
    KW.$("[data-map-line]", fold).textContent = KW.plural(data.nodes.length, "object", "objects") +
      " around this issue" + (failing > 1 ? ", " + KW.num(failing) + " with an open issue." : ".");
    picture(canvas, data, inc.id);
    list(box, data, inc.id);
  };
})();
