/* kwatch dashboard: the parts of "What kwatch found" that live in their
   own tab, drawn after the issue page renders: the errors-per-minute
   strip (Proof > Logs), the possible causes table (Proof > What kwatch
   checked) and the per-pod "read it / did not" fold under
   the changes (Timeline > What changed just before). Read-only, visible
   text first; the terms reuse round.js tooltips. Every string is set
   with textContent. */
(function () {
  "use strict";
  var KW = window.KW;
  var MARK = "data-rd-more";

  function host(parent, before, cls) {
    var h = KW.el("div", cls);
    h.setAttribute(MARK, "");
    parent.insertBefore(h, before || null);
    return h;
  }

  function clear(root) {
    KW.$$("[" + MARK + "]", root).forEach(function (n) { n.parentNode.removeChild(n); });
  }

  function num(v) { return (Math.round(v * 10) / 10).toLocaleString("en-US"); }

  function headRow(table, names) {
    var head = table.appendChild(KW.el("thead")).appendChild(KW.el("tr"));
    names.forEach(function (x) { head.appendChild(KW.el("th", "", x)).scope = "col"; });
  }

  /* strip: errors per minute now against usual, as a table with the
     numbers and a bar whose dashed rule marks "usual". */
  function strip(root, b) {
    var fold = KW.$('[data-fold="logs"]', root);
    if (!b || !fold || fold.hidden) return;
    var body = KW.$(".fold-body", fold);
    var h = host(body, body.firstChild, "rd-strip");
    h.appendChild(KW.el("h3", "fold-h", "Errors per minute"));
    h.appendChild(KW.el("p", "fold-cap", "The last " + KW.plural(b.windowMin, "minute", "minutes") +
      " of this workload's log, against the hour before."));
    var t = h.appendChild(KW.el("table", "rd-table"));
    headRow(t, ["When", "Errors per minute"]);
    var tb = t.appendChild(KW.el("tbody"));
    [["Now", b.perMinute], ["Usual", b.usual]].forEach(function (r) {
      var tr = tb.appendChild(KW.el("tr"));
      tr.appendChild(KW.el("th", "", r[0])).scope = "row";
      tr.appendChild(KW.el("td", "rd-num", num(r[1])));
    });
    var top = Math.max(b.perMinute, b.usual, 0.1);
    var track = h.appendChild(KW.el("div", "rd-track"));
    track.setAttribute("aria-hidden", "true");
    var bar = track.appendChild(KW.el("span", "rd-bar"));
    bar.style.width = Math.max(2, b.perMinute / top * 100) + "%";
    var rule = track.appendChild(KW.el("span", "rd-rule"));
    rule.style.left = (b.usual / top * 100) + "%";
    h.appendChild(KW.el("p", "rd-scale", "Bar: now. Dashed rule: usual (" + num(b.usual) + ")."));
  }

  var STATE = { kept: "Kept", "ruled out": "Ruled out", close: "Close call" };

  /* causes: the stated cause and its rival, with score and proofs. */
  function causes(root, r) {
    var fold = KW.$('[data-fold="why"]', root);
    if (!fold || !r.candidates || !r.candidates.length) return;
    var body = KW.$(".fold-body", fold);
    var h = host(body, KW.$("[data-evidence]", body), "rd-causes");
    h.appendChild(KW.el("h3", "fold-h", "Possible causes"));
    var intro = h.appendChild(KW.el("p", "fold-cap"));
    intro.appendChild(document.createTextNode("Two causes scored close to each other. "));
    if (r.check) {
      KW.roundTerm(intro, "kwatch checked");
      intro.appendChild(document.createTextNode(" to tell them apart: " +
        r.check.text.replace(/^kwatch checked:? ?/, "")));
    } else {
      intro.appendChild(document.createTextNode("kwatch had no check that could separate them."));
    }
    var t = h.appendChild(KW.el("table", "rd-table rd-causes-t"));
    headRow(t, ["Cause", "Score", "Status", "Proof"]);
    var tb = t.appendChild(KW.el("tbody"));
    r.candidates.forEach(function (c) {
      var tr = tb.appendChild(KW.el("tr"));
      var th = tr.appendChild(KW.el("th"));
      th.scope = "row";
      th.appendChild(KW.el("span", "", KW.entityText(c.cause))).title = c.cause;
      tr.appendChild(KW.el("td", "rd-num", c.score + "%"));
      tr.appendChild(KW.el("td", "", STATE[c.state] || c.state));
      var td = tr.appendChild(KW.el("td"));
      if (!(c.proofs || []).length) { td.textContent = "None recorded"; return; }
      var ul = td.appendChild(KW.el("ul", "rd-proofs"));
      c.proofs.forEach(function (p) {
        ul.appendChild(KW.el("li", "", (p.supports ? "For: " : "Against: ") + p.text));
      });
    });
  }

  /* pods: "Read it: 9, did not: 1", folded, closed by default; each pod
     opens Proof > Containers. */
  function pods(root, r) {
    var b = r.blast;
    var fold = KW.$('[data-fold="changes"]', root);
    if (!b || !b.pods || !b.pods.length || !fold || fold.hidden) return;
    var read = b.pods.filter(function (p) { return p.read; });
    var other = b.pods.filter(function (p) { return !p.read; });
    var h = host(KW.$("[data-changes]", root), null, "rd-pods");
    var p = h.appendChild(KW.el("p", "rd-line"));
    KW.roundTerm(p, "failing pods read it", b.count + " of " + b.total + " failing pods " + b.verb + " it");
    p.appendChild(document.createTextNode("."));
    var d = h.appendChild(KW.el("details", "rd-fold"));
    d.appendChild(KW.el("summary", "", "Read it: " + read.length + ", did not: " + other.length));
    [["Read it", read], ["Did not", other]].forEach(function (g) {
      if (!g[1].length) return;
      d.appendChild(KW.el("h4", "rd-sub", g[0]));
      var ul = d.appendChild(KW.el("ul", "rd-podlist"));
      g[1].forEach(function (x) {
        var a = ul.appendChild(KW.el("li")).appendChild(KW.el("button", "link mono", x.name));
        a.type = "button";
        a.title = "Open Containers";
        a.onclick = function () { KW.openFold(root, "containers"); };
      });
    });
  }

  KW.roundMore = function (root, inc) {
    clear(root);
    var r = inc.round;
    if (!r) return;
    strip(root, r.burst);
    causes(root, r);
    pods(root, r);
  };
})();
