/* kwatch dashboard: the release linked to an issue (inc.release, from
   the API's release book). The release is told in Why? and What should
   I do? on the summary (incsum.js); this file words the release as the
   cause when no other cause is known. The dashboard never changes the
   cluster: the undo is a command to copy. */
(function () {
  "use strict";
  var KW = window.KW;

  /* KW.releaseWhy: the line for Why? when a release that went wrong is
     linked and no other cause is known, or "". */
  var ACTS = { failed: "fails.", down: "has no replica ready.",
    regressed: "restarts far more than the previous one." };
  /* KW.sourceLinks: "Source: Argo CD app shop · commit 1a2b3c4", where
     the change came from (links from the API, built only from what the
     objects recorded and app.gitOpsLinks). A link without an address,
     or one that is not http(s), is text. null without links. */
  KW.sourceLinks = function (links, cls) {
    if (!links || !links.length) return null;
    var p = KW.el("p", cls || "src-links");
    p.appendChild(document.createTextNode("Source: "));
    links.forEach(function (l, i) {
      if (i) p.appendChild(document.createTextNode(" · "));
      var url = String(l.url || ""), scheme = url.slice(0, url.indexOf(":") + 1).toLowerCase();
      if (scheme !== "https:" && scheme !== "http:") {
        p.appendChild(document.createTextNode(l.label));
        return;
      }
      var a = p.appendChild(KW.el("a", "ulink", l.label));
      a.href = url;
      a.target = "_blank";
      a.rel = "noopener noreferrer";
    });
    return p;
  };

  KW.releaseWhy = function (inc) {
    var r = inc.release;
    if (!r || !ACTS[r.verdict] || !KW.valid(r.at)) return "";
    return "The " + r.name + " release rolled out " + KW.when(r.at) + " and the new version " +
      ACTS[r.verdict];
  };
})();
