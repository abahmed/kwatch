/* kwatch dashboard: "?" opens and closes the help beside the logo,
   which lists the keyboard keys; Esc, a click outside it or moving to
   another page closes it. The other keys
   (/, j, k) are in main.js. */
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
