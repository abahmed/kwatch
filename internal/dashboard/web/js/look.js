/* kwatch dashboard: text size, high contrast and theme, set on <html> before
   the page draws. This file loads first and is not deferred, so the
   page never flashes at the wrong size. It reads the view options
   (localStorage key kwatch.view: "text" is s, m, l or xl,
   "contrast" is "more" and "theme" light or dark, else the system's)
   and settings.js keeps KW.look to apply a change. */
(function () {
  "use strict";
  var SIZES = ["s", "m", "l", "xl"];

  var THEMES = ["system", "light", "dark"];

  /* look sets the attributes look.css and tokens.css read; normal text,
     normal contrast and the system's theme set none. A theme left out
     (undefined) stays as it is. */
  function look(text, contrast, theme) {
    var root = document.documentElement;
    if (SIZES.indexOf(text) >= 0 && text !== "m") root.setAttribute("data-text", text);
    else root.removeAttribute("data-text");
    if (contrast === "more") root.setAttribute("data-contrast", "more");
    else root.removeAttribute("data-contrast");
    if (theme === undefined) return;
    if (theme === "light" || theme === "dark") root.setAttribute("data-theme", theme);
    else root.removeAttribute("data-theme");
  }

  var v = null;
  try { v = JSON.parse(window.localStorage.getItem("kwatch.view") || "null"); } catch (e) { v = null; }
  if (v && typeof v === "object") look(v.text, v.contrast, v.theme || "system");
  window.KW = { look: look, TEXT_SIZES: SIZES, THEMES: THEMES };
})();
