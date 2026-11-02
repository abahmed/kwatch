/* kwatch dashboard: the quick views above the Issues table ("New
   today", ...), this browser's saved views beside them, and the "3 new
   since your last visit" line. A quick view only sets filters, so it
   shows in the address like any other filter; a second click on the
   one shown clears it ("Clear all" under the filters does too).
   issues.js builds it with quickViews(box, onPick). */
(function () {
  "use strict";
  var KW = window.KW;
  var LQ = function () { return KW.listQuery; };

  function chip(text, title, cls) {
    var b = KW.el("button", "qv" + (cls ? " " + cls : ""), text);
    b.type = "button";
    b.title = title;
    b.setAttribute("aria-pressed", "false");
    return b;
  }

  /* quickViews fills box. onPick(view) shows a view. It returns
     paint(view) to highlight the quick or saved view shown, and
     news(text, onMarkAll) for the line about what changed. */
  KW.quickViews = function (box, onPick) {
    var current = null;
    var row = KW.el("div", "qv-row");
    row.setAttribute("role", "group");
    row.setAttribute("aria-label", "Quick views");
    row.appendChild(KW.el("span", "f-lbl", "Quick views"));
    var buttons = LQ().PRESETS.map(function (pr) {
      var b = chip(pr.label, pr.title);
      b.dataset.qv = pr.id;
      b.addEventListener("click", function () {
        // A second click on the view shown clears it.
        var on = b.getAttribute("aria-pressed") === "true";
        onPick(on ? clearOf(current) : LQ().presetState(pr, current));
      });
      row.appendChild(b);
      return b;
    });
    var saved = KW.el("span", "qv-saved");
    row.appendChild(saved);
    box.appendChild(row);

    function clearOf(f) {
      var next = LQ().blank();
      if (f) ["sort", "dir", "pageSize"].forEach(function (k) { next[k] = f[k]; });
      return next;
    }

    function key(f) { return LQ().search(Object.assign({}, f, { page: 1 })); }

    /* plain: a view that changes nothing (no filter, the default
       status and order): it never shows as the one in use. */
    function plain(v) {
      return !LQ().used(v) && v.status === "active" && !v.sort;
    }

    /* views draws the saved views as chips, the default one marked. */
    function views(f) {
      saved.textContent = "";
      var here = key(f);
      KW.prefs.views().forEach(function (v) {
        var b = chip(v.name + (v.isDefault ? " · default" : ""),
          "Your saved view" + (v.isDefault ? ", opened by default" : "") + ".", "qv-mine");
        var view = LQ().parse(v.query);
        b.setAttribute("aria-pressed", key(view) === here && !plain(view) ? "true" : "false");
        b.addEventListener("click", function () {
          KW.prefs.useView(v);
          onPick(LQ().parse(v.query));
        });
        saved.appendChild(b);
      });
    }

    var line = KW.el("p", "qv-news");
    line.setAttribute("role", "status");
    line.hidden = true;
    var text = KW.el("span");
    var mark = KW.el("button", "link", "Mark all seen");
    mark.type = "button";
    line.appendChild(text);
    line.appendChild(document.createTextNode(" · "));
    line.appendChild(mark);
    box.appendChild(line);
    var onMark = null;
    mark.addEventListener("click", function () {
      if (!onMark) return;
      onMark();
      KW.announce("Marked all as seen.");
    });

    return {
      paint: function (f) {
        current = f;
        var id = LQ().matchPreset(f);
        buttons.forEach(function (b) {
          b.setAttribute("aria-pressed", b.dataset.qv === id ? "true" : "false");
        });
        views(f);
      },
      news: function (words, markAll) {
        onMark = markAll;
        line.hidden = !words;
        text.textContent = words;
        var at = KW.visits && KW.visits.lastVisit();
        text.title = at ? "Your last visit: " + KW.full(new Date(at).toISOString()) : "";
      }
    };
  };
})();
