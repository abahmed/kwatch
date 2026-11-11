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
  KW.releaseWhy = function (inc) {
    var r = inc.release;
    if (!r || !ACTS[r.verdict] || !KW.valid(r.at)) return "";
    return "The " + r.name + " release rolled out " + KW.when(r.at) + " and the new version " +
      ACTS[r.verdict];
  };
})();
