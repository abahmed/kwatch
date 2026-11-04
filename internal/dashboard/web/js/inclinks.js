/* kwatch dashboard: how an issue relates to the others. "Caused by"
   and the moved links in the summary card; in the Impact tab "Who it
   hits" (the blast radius as count chips) and "Other issues": "Also
   causing" (the issues this one causes) and "Started at the same time"
   (issues that started within 5 minutes of it, less those already
   named). Every part hides when the API leaves its field out. */
(function () {
  "use strict";
  var KW = window.KW;

  var RELATION = {
    "same node": ["Same node", "Runs on the same node as this issue."],
    "same namespace": ["Same namespace", "In the same namespace as this issue."],
    cluster: ["Elsewhere", "Elsewhere in the cluster, at the same moment."]
  };

  /* refs: a field that names other issues, as a list of {id, title,
     severity, state}; an ID alone, one object or a list are accepted. */
  function refs(v) {
    if (!v) return [];
    if (!Array.isArray(v)) v = [v];
    return v.map(function (x) {
      return typeof x === "string" ? { id: x, title: x } : x;
    }).filter(function (x) { return x && (x.id || x.title); });
  }

  function link(r) {
    var label = r.title || r.summary || r.id;
    if (!r.id) return KW.el("span", "", label);
    var a = KW.el("a", "ulink", label);
    a.href = "/issues/" + encodeURIComponent(r.id);
    a.dataset.link = "";
    return a;
  }

  /* blastText: the blast radius as one line; a sentence when the API
     gives one, else its counts ("3 pods · 2 services"). */
  function blastText(b) {
    if (!b) return "";
    if (typeof b === "string") return b;
    var text = b.text || b.summary || b.sentence;
    if (typeof text === "string") return text;
    return Object.keys(b).filter(function (k) {
      return typeof b[k] === "number" && b[k] > 0;
    }).map(function (k) { return b[k] + " " + k; }).join(" · ");
  }

  function causedBy(root, inc) {
    var p = KW.$("[data-caused]", root), list = refs(inc.causedBy);
    p.textContent = "";
    p.hidden = !list.length;
    if (p.hidden) return;
    p.appendChild(KW.el("b", "", "Caused by: "));
    list.forEach(function (r, i) {
      if (i) p.appendChild(document.createTextNode(", "));
      p.appendChild(link(r));
    });
    var cut = KW.cutCount(inc, "causedBy");
    if (cut) p.appendChild(document.createTextNode(" (" + KW.num(cut) + " more not shown)"));
    p.title = "kwatch thinks this issue is a result of that one. Fix that first.";
  }

  /* moved: when kwatch folded this issue into another one, or folded
     others into this one. Only issues still kept are named. */
  function moved(root, inc) {
    var box = KW.$("[data-moved]", root), m = inc.moved || {};
    box.textContent = "";
    var from = refs(m.from), cut = KW.cutCount(inc, "movedFrom");
    box.hidden = !m.into && !from.length;
    if (m.into) {
      var p = box.appendChild(KW.el("p"));
      p.appendChild(KW.el("b", "", "Moved into: "));
      p.appendChild(link(m.into));
    }
    if (!from.length) return;
    var q = box.appendChild(KW.el("p"));
    q.appendChild(KW.el("b", "", "Includes earlier issues: "));
    from.forEach(function (r, i) {
      if (i) q.appendChild(document.createTextNode(", "));
      q.appendChild(link(r));
    });
    if (cut) q.appendChild(document.createTextNode(" (" + KW.num(cut) + " more not shown)"));
  }

  /* chips: the blast radius as counts, each with its words. */
  function chips(inc) {
    var out = [], by = {};
    (inc.affected || []).forEach(function (o) { by[o.status] = (by[o.status] || 0) + 1; });
    [["failing", "failing object", "failing objects", "c-page"], ["cause", "likely cause", "likely causes", "c-notify"],
      ["affected", "object hit", "objects hit", "c-notify"], ["gone", "deleted object", "deleted objects", "c-low"]
    ].forEach(function (k) { if (by[k[0]]) out.push([KW.plural(by[k[0]], k[1], k[2]), k[3]]); });
    var down = (inc.traffic || []).filter(function (t) { return t.total && t.ready < t.total; }).length;
    if (down) out.push([KW.plural(down, "Service or Ingress short of endpoints", "Services or Ingresses short of endpoints"), "c-page"]);
    var causing = refs(inc.causing).length + KW.cutCount(inc, "causing");
    if (causing) out.push([KW.plural(causing, "issue it causes", "issues it causes"), "c-notify"]);
    var same = (inc.sameTime || []).length;
    if (same) out.push([KW.plural(same, "issue at the same time", "issues at the same time"), "c-low"]);
    return out;
  }

  function blast(root, inc) {
    var p = KW.$("[data-blast]", root), text = blastText(inc.blastRadius);
    var box = KW.$("[data-blast-chips]", root), list = chips(inc);
    p.textContent = text;
    p.hidden = !text;
    box.textContent = "";
    list.forEach(function (c) { box.appendChild(KW.chip(c[1] + " bl-chip", c[0])); });
    box.hidden = !list.length;
    KW.$('[data-fold="blast"]', root).hidden = !text && !list.length;
  }

  function row(r, meta) {
    var li = KW.el("li", "rel-row");
    if (r.severity) li.appendChild(KW.severityChip(r.severity));
    var body = li.appendChild(KW.el("div", "rel-body"));
    body.appendChild(KW.el("p")).appendChild(link(r));
    if (meta) body.appendChild(KW.el("small", "cx-meta", meta));
    return li;
  }

  function causing(box, list, inc) {
    box.textContent = "";
    if (!list.length) return "";
    box.appendChild(KW.el("h3", "fold-h", "Also causing"));
    box.appendChild(KW.el("p", "", "kwatch thinks these issues are a result of this one. They may clear once it is fixed."));
    var ul = box.appendChild(KW.el("ul", "rel-list"));
    list.forEach(function (r) { ul.appendChild(row(r, r.namespace || "")); });
    var cut = KW.cutCount(inc, "causing");
    if (cut) box.appendChild(KW.el("p", "ev-cap", KW.plural(cut, "more issue", "more issues") + " not shown."));
    return "Also causing: " + KW.plural(list.length, "issue", "issues");
  }

  /* sameTime lists the issues that started within 5 minutes, except
     those already named as caused by or causing this one. */
  function sameTime(box, inc, known) {
    var list = (inc.sameTime || []).filter(function (s) { return !known[s.id]; });
    var cut = KW.cutCount(inc, "sameTime");
    box.textContent = "";
    if (!list.length) return ["", 0];
    var total = list.length + cut;
    var lead = (total === 1 ? "1 other issue" : KW.num(total) + " other issues") +
      " started within 5 minutes of this one";
    box.appendChild(KW.el("h3", "fold-h", "Started at the same time"));
    box.appendChild(KW.el("p", "", lead + ". Issues that start together often share a cause."));
    var ul = box.appendChild(KW.el("ul", "rel-list"));
    list.forEach(function (s) {
      var rel = RELATION[s.relation] || [s.relation, ""];
      var start = Date.parse(s.startedAt) - Date.parse(inc.firstSeen);
      var gap = Math.abs(start) < 60000 ? "started at the same minute" :
        KW.lasted(start < 0 ? s.startedAt : inc.firstSeen, start < 0 ? inc.firstSeen : s.startedAt) +
        (start < 0 ? " before" : " after");
      var li = ul.appendChild(row(s, [rel[0], s.namespace, gap].filter(Boolean).join(" · ")));
      li.title = rel[1];
    });
    if (cut) box.appendChild(KW.el("p", "ev-cap", KW.plural(cut, "more issue", "more issues") + " not shown."));
    return [lead, total];
  }

  /* others fills "Other issues": "Also causing", then "Started at the
     same time". It returns the hint lines for the summary card. */
  function others(root, inc) {
    var list = refs(inc.causing), known = {};
    list.concat(refs(inc.causedBy)).forEach(function (r) { if (r.id) known[r.id] = true; });
    var c = causing(KW.$("[data-causing]", root), list, inc);
    var s = sameTime(KW.$("[data-sametime]", root), inc, known);
    var n = list.length + KW.cutCount(inc, "causing") + s[1];
    KW.$('[data-fold="others"]', root).hidden = !n;
    KW.$("[data-others-n]", root).textContent = n ? "(" + KW.num(n) + ")" : "";
    return [c, s[0]];
  }

  /* KW.links fills "Caused by" and the moved links of the summary,
     and "Who it hits" and "Other issues" in the Impact tab. */
  KW.links = function (root, inc) {
    causedBy(root, inc);
    blast(root, inc);
    moved(root, inc);
    others(root, inc);
  };
})();
