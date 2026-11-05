/* kwatch dashboard: the Map page (/map), how objects connect and where
   it fails, from /api/map. One namespace at a time (?namespace=), or
   what fails across the watched namespaces with what is next to it
   (?view=failing); nodes on request (?nodes=1). Routes, Services and
   workloads (pods folded in) are boxes colored by the worst open issue
   touching them, laid out left to right by maplayout.js and drawn as
   SVG; a box with an issue opens it, any other shows its status below.
   The same objects and connections are a plain list under the drawing,
   which is all a narrow screen shows. Reads with its ETag. */
(function () {
  "use strict";
  var KW = window.KW;
  var SVG = "http://www.w3.org/2000/svg";
  var LEVEL = { page: "Urgent", notify: "Needs attention", digest: "Low", ok: "No open issue" };
  var EDGE = { routes: "routes to", selects: "sends to", calls: "calls", "runs-on": "runs on" };

  function svg(tag, attrs, cls) {
    var e = document.createElementNS(SVG, tag);
    Object.keys(attrs || {}).forEach(function (k) { e.setAttribute(k, attrs[k]); });
    if (cls) e.setAttribute("class", cls);
    return e;
  }

  function label(n) { return n.kind + " " + (n.namespace ? n.namespace + "/" : "") + n.name; }

  function short(text, max) { return text.length > max ? text.slice(0, max - 1) + "…" : text; }

  function params() {
    var p = new URLSearchParams(location.search);
    return { view: p.get("view") === "failing" ? "failing" : "namespace",
      namespace: p.get("namespace") || "", nodes: p.get("nodes") === "1" };
  }

  function query(s) {
    var q = new URLSearchParams();
    if (s.view === "failing") q.set("view", "failing");
    else if (s.namespace) q.set("namespace", s.namespace);
    if (s.nodes) q.set("nodes", "1");
    return q.toString();
  }

  /* draw renders data into the canvas, the list and the line above. */
  function draw(box, data, open) {
    box.canvas.textContent = "";
    box.list.textContent = "";
    box.info.hidden = true;
    var nodes = data.nodes || [], edges = data.edges || [], byId = {};
    nodes.forEach(function (n) { byId[n.id] = n; });
    var failing = nodes.filter(function (n) { return n.level !== "ok"; }).length;
    if (data.tooMany) {
      box.line.textContent = "Too many objects to draw (" + KW.num(data.count) +
        "). Pick a namespace.";
      return;
    }
    box.line.textContent = !nodes.length ? (data.view === "failing" ?
      "Nothing is failing." : "Nothing to draw in this namespace.") :
      KW.plural(nodes.length, "object", "objects") + ", " +
      (failing ? KW.num(failing) + " with an open issue." : "none with an open issue.");
    if (!nodes.length) return;
    var lay = KW.mapLayout(nodes, edges);
    var root = box.canvas.appendChild(svg("svg", { viewBox: "0 0 " + lay.width + " " + lay.height,
      width: lay.width, height: lay.height, role: "group", "aria-label": "Map of " +
      KW.plural(nodes.length, "object", "objects") }, "map-svg"));
    edges.forEach(function (e) {
      var a = lay.pos[e.from], b = lay.pos[e.to];
      if (!a || !b) return;
      var x1 = a.x + lay.w, y1 = a.y + lay.h / 2, x2 = b.x, y2 = b.y + lay.h / 2, mid = (x1 + x2) / 2;
      root.appendChild(svg("path", { d: "M" + x1 + " " + y1 + " C" + mid + " " + y1 + " " + mid +
        " " + y2 + " " + x2 + " " + y2, "aria-hidden": "true" }, "map-edge map-e-" + e.type));
    });
    nodes.forEach(function (n) {
      var p = lay.pos[n.id];
      var g = root.appendChild(svg("g", { transform: "translate(" + p.x + " " + p.y + ")",
        tabindex: "0", role: "button", "aria-label": label(n) + ", " + LEVEL[n.level] +
        (n.ready ? ", " + n.ready + " ready" : "") }, "map-node map-l-" + n.level +
        (n.external ? " map-ext" : "")));
      g.appendChild(svg("title")).textContent = label(n);
      g.appendChild(svg("rect", { width: lay.w, height: lay.h, rx: 6 }));
      g.appendChild(svg("text", { x: 10, y: 15 }, "map-kind")).textContent = n.kind +
        (n.ready ? " · " + n.ready : "");
      g.appendChild(svg("text", { x: 10, y: 29 }, "map-name")).textContent = short(n.name, 24);
      var act = function () { open(n); };
      g.addEventListener("click", act);
      g.addEventListener("keydown", function (ev) {
        if (ev.key === "Enter" || ev.key === " ") { ev.preventDefault(); act(); }
      });
    });
    nodes.forEach(function (n) {
      var li = box.list.appendChild(KW.el("li"));
      var name = li.appendChild(KW.el(n.issue ? "a" : "span", "map-li", label(n)));
      if (n.issue) {
        name.href = "/issues/" + encodeURIComponent(n.issue);
        name.dataset.link = "";
      }
      li.appendChild(KW.el("span", "map-li-level map-l-" + n.level, LEVEL[n.level]));
      var out = edges.filter(function (e) { return e.from === n.id && byId[e.to]; });
      if (out.length) {
        li.appendChild(KW.el("span", "map-li-out", out.map(function (e) {
          return EDGE[e.type] + " " + label(byId[e.to]);
        }).join("; ")));
      }
    });
  }

  KW.mapView = function (main) {
    var state = params(), etag = "", seq = 0;
    var pg = KW.pageView(main, { title: "Map", tab: "Map · kwatch", read: load,
      sub: "How Ingresses, Services and workloads connect, and where it is failing." });
    var bar = pg.body.appendChild(KW.el("div", "map-bar"));
    var sw = bar.appendChild(KW.el("div", "tz"));
    sw.setAttribute("role", "group");
    sw.setAttribute("aria-label", "Show");
    var views = [["namespace", "One namespace"], ["failing", "What's failing"]].map(function (v) {
      var b = sw.appendChild(KW.el("button", "tz-b", v[1]));
      b.type = "button";
      b.dataset.mapView = v[0];
      return b;
    });
    var nsLabel = bar.appendChild(KW.el("label", "map-ns"));
    nsLabel.appendChild(KW.el("span", "", "Namespace"));
    var pick = nsLabel.appendChild(KW.el("select"));
    pick.name = "map-namespace";
    var nodesLabel = bar.appendChild(KW.el("label", "map-nodes"));
    var nodesBox = nodesLabel.appendChild(KW.el("input"));
    nodesBox.type = "checkbox";
    nodesBox.name = "map-nodes";
    nodesLabel.appendChild(document.createTextNode("Show nodes"));
    var box = { line: pg.body.appendChild(KW.el("p", "map-line")) };
    box.canvas = pg.body.appendChild(KW.el("div", "map-canvas"));
    box.info = pg.body.appendChild(KW.el("div", "map-info"));
    box.info.setAttribute("role", "status");
    box.info.hidden = true;
    pg.body.appendChild(KW.el("h2", "map-h", "Objects and connections"));
    box.list = pg.body.appendChild(KW.el("ul", "map-list"));
    pg.body.hidden = true;

    function open(n) {
      if (n.issue) {
        KW.go("/issues/" + encodeURIComponent(n.issue));
        return;
      }
      box.info.textContent = label(n) + ": " + LEVEL[n.level] +
        (n.ready ? ", " + n.ready + " ready" : "") + ".";
      box.info.hidden = false;
    }

    function mark() {
      views.forEach(function (b) {
        b.setAttribute("aria-pressed", b.dataset.mapView === state.view ? "true" : "false");
      });
      nsLabel.hidden = state.view === "failing";
      nodesBox.checked = state.nodes;
    }

    function names(list, current) {
      var want = ["", current].concat(list).join("\n");
      if (pick.dataset.names === want) return;
      pick.dataset.names = want;
      pick.textContent = "";
      list.forEach(function (ns) {
        var o = pick.appendChild(KW.el("option", "", ns));
        o.value = ns;
      });
      pick.value = current;
    }

    function load() {
      var mine = ++seq;
      return KW.api("/api/map?" + query(state), { etag: etag }).then(function (res) {
        if (mine !== seq) return null;
        if (res.status === 200 && res.data) {
          etag = res.etag;
          if (state.view === "namespace" && res.data.namespace) state.namespace = res.data.namespace;
          names(res.data.namespaces || [], state.namespace);
          draw(box, res.data, open);
        }
        return res;
      });
    }

    function change() {
      etag = "";
      mark();
      var q = query(state);
      history.replaceState(null, "", "/map" + (q ? "?" + q : ""));
      KW.rendered = location.pathname + location.search;
      pg.now();
    }

    views.forEach(function (b) {
      b.addEventListener("click", function () { state.view = b.dataset.mapView; change(); });
    });
    pick.addEventListener("change", function () { state.namespace = pick.value; change(); });
    nodesBox.addEventListener("change", function () { state.nodes = nodesBox.checked; change(); });
    mark();
    return { stop: pg.stop };
  };
})();
