/* kwatch dashboard: text size and high contrast, set on <html> before
   the page draws. This file loads first and is not deferred, so the
   page never flashes at the wrong size. It reads the view options
   (localStorage key kwatch.view: "text" is s, m, l or xl and
   "contrast" is "more") and core.js keeps KW.look to apply a change. */
(function () {
  "use strict";
  var SIZES = ["s", "m", "l", "xl"];

  /* look sets the attributes look.css reads; normal text and normal
     contrast set none. */
  function look(text, contrast) {
    var root = document.documentElement;
    if (SIZES.indexOf(text) >= 0 && text !== "m") root.setAttribute("data-text", text);
    else root.removeAttribute("data-text");
    if (contrast === "more") root.setAttribute("data-contrast", "more");
    else root.removeAttribute("data-contrast");
  }

  var v = null;
  try { v = JSON.parse(window.localStorage.getItem("kwatch.view") || "null"); } catch (e) { v = null; }
  if (v && typeof v === "object") look(v.text, v.contrast);
  window.KW = { look: look, TEXT_SIZES: SIZES };
})();
