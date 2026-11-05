/* kwatch dashboard: "?" opens and closes the help beside the logo,
   which lists the keyboard keys; Esc, a click outside it or moving to
   another page closes it. "g" then a letter goes to a page: o
   Overview, i Issues, r Releases, m Map, a Activity, k kwatch. The other keys
   (/, j, k) are in main.js; this file's listener runs first, so the
   letter after "g" does not also move through a list. */
(function () {
  "use strict";

  function typing(t) {
    return t && (t.isContentEditable ||
      /^(INPUT|SELECT|TEXTAREA)$/.test(t.tagName));
  }

  /* closeHelp shuts the help without moving the focus. */
  function closeHelp() {
    var help = document.querySelector(".legend");
    if (help) help.open = false;
  }
  document.addEventListener("click", function (e) {
    var help = document.querySelector(".legend");
    if (help && help.open && !help.contains(e.target)) closeHelp();
  });
  window.addEventListener("popstate", closeHelp);
  window.addEventListener("hashchange", closeHelp);

  var GO = { o: "/", i: "/issues", r: "/releases", m: "/map", a: "/activity", k: "/kwatch" };
  var gAt = 0;
  document.addEventListener("keydown", function (e) {
    if (e.metaKey || e.ctrlKey || e.altKey || typing(e.target)) return;
    if (gAt && Date.now() - gAt < 1500 && GO[e.key]) {
      gAt = 0;
      e.preventDefault();
      e.stopImmediatePropagation();
      if (document.querySelector("[data-drawer]:not([hidden])")) return;
      closeHelp();
      if (location.pathname !== GO[e.key] && window.KW.go) window.KW.go(GO[e.key]);
      return;
    }
    gAt = e.key === "g" ? Date.now() : 0;
  });

  document.addEventListener("keydown", function (e) {
    var help = document.querySelector(".legend");
    if (!help) return;
    var summary = help.querySelector("summary");
    if (e.key === "Escape" && help.open) {
      help.open = false;
      summary.focus();
      return;
    }
    if (e.key !== "?" || e.metaKey || e.ctrlKey || e.altKey ||
        typing(e.target)) {
      return;
    }
    e.preventDefault();
    help.open = !help.open;
    if (help.open) summary.focus();
  });
})();
