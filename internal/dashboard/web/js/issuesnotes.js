/* kwatch dashboard: the short notes of the Issues page: the line under
   the title ("23 issues happening now · 0 match"), what a refused search
   says (a 400 with its reason and the word it choked on) and the note
   that the rows shown are of the last list. issues.js uses them. */
(function () {
  "use strict";
  var KW = window.KW;
  var LQ = function () { return KW.listQuery; };

  /* subtitle: how many issues there are. While filters hide some, it
     still says how many are happening now, then how many match. */
  function subtitle(f, data) {
    var rest = f.status === "resolved" ? " · kwatch keeps resolved issues for 7 days" : "";
    if (LQ().used(f)) {
      if (f.status === "active" && KW.activeTotal !== undefined) {
        return KW.plural(KW.activeTotal, "issue", "issues") + " happening now · " +
          KW.num(data.total) + " match";
      }
      return KW.plural(data.total, "issue", "issues") + " match" + rest;
    }
    return KW.plural(data.total, "issue", "issues") +
      (f.status === "active" ? " happening now" : f.status === "resolved" ?
        " resolved · kwatch keeps them for 7 days" : " in the last 7 days");
  }

  /* badRequest: the server's plain sentence (res.data.error) with the
     word it choked on (term) marked, and "Clear filters". */
  function badRequest(res, onClear) {
    var d = res.data || {}, term = String(d.term || "");
    var box = KW.el("div", "badreq");
    box.setAttribute("role", "alert");
    var p = box.appendChild(KW.el("p"));
    // A term that is only the name of a setting ("q") is not a word.
    var low = [term.toLowerCase()].filter(function (t) { return t && t !== d.param; });
    KW.highlight(p, d.error || "kwatch can't use this search.", low);
    if (low.length && !p.querySelector("mark")) {
      p.appendChild(document.createTextNode(" From: "));
      p.appendChild(KW.el("mark", "hl", term));
    }
    var b = box.appendChild(KW.el("button", "btn btn-sm", "Clear filters"));
    b.type = "button";
    b.addEventListener("click", onClear);
    return box;
  }

  /* stale: the reader changed the view but the list could not be read,
     so the rows are of the view before. */
  function stale() {
    var n = KW.el("p", "notice n-degraded",
      "Showing the last list. This filter applies when kwatch is reachable.");
    n.setAttribute("role", "status");
    return n;
  }

  KW.listNotes = { subtitle: subtitle, badRequest: badRequest, stale: stale };
})();
