/* kwatch dashboard: the display part of Settings, on its own page
   (/settings, "Settings" in the side bar): the theme (System, Light,
   Dark), the text size and high contrast. Each applies at once
   (look.js) and is kept in this browser (KW.store, key kwatch.view:
   theme, text, contrast); a browser that refuses to keep it still
   shows the choice until a reload. */
(function () {
  "use strict";
  var KW = window.KW;

  function theme() {
    var t = KW.store.get("theme");
    return KW.THEMES.indexOf(t) >= 0 ? t : "system";
  }

  /* KW.wireSettings wires the controls of the shell root. redraw() is
     called after a change of text size, which moves layouts. */
  /* The Settings panel lives in the shell, parked out of sight, so its
     controls stay wired; /settings moves it into the page and any
     other page parks it again before the page is cleared. */
  function home(root) { return KW.$("[data-settings-home]", root); }

  KW.parkSettings = function (root) {
    var panel = KW.$("[data-settings]", root);
    if (panel && panel.parentNode !== home(root)) home(root).appendChild(panel);
  };

  KW.settingsView = function (root, main) {
    KW.title("Settings");
    main.appendChild(KW.$("[data-settings]", root));
    return { stop: function () { KW.parkSettings(root); } };
  };

  KW.wireSettings = function (root, redraw) {
    var text = KW.$("[data-look-text]", root);
    var contrast = KW.$("[data-look-contrast]", root);
    var themes = KW.$$("[data-theme-b]", root);
    if (!text || !contrast) return;

    function paint() {
      var o = KW.prefs.options();
      text.value = o.text;
      contrast.checked = o.contrast;
      var t = theme();
      themes.forEach(function (b) {
        b.setAttribute("aria-pressed", b.dataset.themeB === t ? "true" : "false");
      });
    }

    function look() {
      KW.prefs.setLook(text.value, contrast.checked);
      if (redraw) redraw();
    }
    text.addEventListener("change", look);
    contrast.addEventListener("change", look);
    themes.forEach(function (b) {
      b.addEventListener("click", function () {
        var t = b.dataset.themeB;
        KW.store.set("theme", t === "system" ? undefined : t);
        KW.look(text.value, contrast.checked ? "more" : "", t);
        paint();
        KW.announce("Theme: " + b.textContent + ".");
      });
    });
    paint();
  };
})();
