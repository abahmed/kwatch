/* kwatch dashboard: the Map page's layout (map.js draws it). Objects go
   in columns, left to right, by how traffic flows: routes (Ingress,
   HTTPRoute), Services, workloads, what workloads call, nodes last.
   Each object's column is one more than the deepest object pointing at
   it; within a column objects are ordered by the average row of what
   points at them (the barycenter), two sweeps, which keeps crossings
   few without any library. Pure: the same graph gives the same layout. */
(function () {
  "use strict";
  var KW = window.KW;
  var START = { ingress: 0, httproute: 0, service: 1 };
  var COL = 230, ROW = 56, W = 184, H = 36, PAD = 12;

  function columns(nodes, edges) {
    var col = {}, byId = {};
    nodes.forEach(function (n) {
      byId[n.id] = n;
      col[n.id] = n.kind in START ? START[n.kind] : 2;
    });
    // Longest path from the left, bounded so a cycle cannot run away.
    for (var pass = 0; pass < nodes.length; pass++) {
      var moved = false;
      edges.forEach(function (e) {
        if (e.type === "runs-on" || !(e.from in col) || !(e.to in col)) return;
        if (col[e.to] < col[e.from] + 1 && col[e.from] + 1 < 12) {
          col[e.to] = col[e.from] + 1;
          moved = true;
        }
      });
      if (!moved) break;
    }
    var last = 0;
    nodes.forEach(function (n) { if (n.kind !== "node") last = Math.max(last, col[n.id]); });
    nodes.forEach(function (n) { if (n.kind === "node") col[n.id] = last + 1; });
    return col;
  }

  /* KW.mapLayout places nodes: {pos: id → {x, y}, width, height}. */
  KW.mapLayout = function (nodes, edges) {
    var col = columns(nodes, edges), groups = [], row = {};
    nodes.forEach(function (n) {
      (groups[col[n.id]] = groups[col[n.id]] || []).push(n.id);
    });
    groups = groups.filter(Boolean);
    groups.forEach(function (g) {
      g.sort();
      g.forEach(function (id, i) { row[id] = i; });
    });
    var into = {};
    edges.forEach(function (e) { (into[e.to] = into[e.to] || []).push(e.from); });
    for (var sweep = 0; sweep < 2; sweep++) {
      groups.forEach(function (g, gi) {
        if (!gi) return;
        var score = {};
        g.forEach(function (id) {
          var from = (into[id] || []).filter(function (f) { return f in row; });
          score[id] = from.length ? from.reduce(function (s, f) { return s + row[f]; }, 0) / from.length : row[id];
        });
        g.sort(function (a, b) { return score[a] - score[b] || (a < b ? -1 : 1); });
        g.forEach(function (id, i) { row[id] = i; });
      });
    }
    var pos = {}, tall = 0;
    groups.forEach(function (g, gi) {
      g.forEach(function (id, i) { pos[id] = { x: PAD + gi * COL, y: PAD + i * ROW }; });
      tall = Math.max(tall, g.length);
    });
    return { pos: pos, width: PAD * 2 + Math.max(1, groups.length) * COL - (COL - W),
      height: PAD * 2 + Math.max(1, tall) * ROW - (ROW - H), w: W, h: H };
  };
})();
