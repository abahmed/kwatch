/* kwatch dashboard: "View options" on the Issues page. Density, rows
   per page, text size, high contrast, what a bare /issues opens with (status,
   order, or a saved view) and the saved views: save, open, rename,
   delete and pick the default. The columns have their own panel
   (columns.js), which a link here opens; Local or UTC times are in the
   sidebar.
   Everything is kept in this browser (prefs.js). */
(function () {
  "use strict";
  var KW = window.KW;
  var SORTS = [["", "Most urgent, then newest"], ["severity", "Severity"],
    ["last", "Latest activity first"], ["started", "Newest first"],
    ["duration", "Longest first"], ["repeats", "Most repeated first"],
    ["namespace", "Namespace A to Z"], ["title", "Title A to Z"]];

  function option(sel, value, text) {
    sel.appendChild(KW.el("option", "", text)).value = value;
  }

  /* viewOptions wires the popover box. hooks: view() is the view shown,
     redraw() draws it again, open(query) shows another view, saved()
     says the saved views changed, columns() opens the Columns panel. */
  KW.viewOptions = function (box, hooks) {
    var $ = function (sel) { return KW.$(sel, box); };
    var P = KW.prefs, LQ = KW.listQuery;
    var list = $("[data-vo-views]");

    function paint() {
      var o = P.options();
      $("[data-vo-density]").value = o.density;
      $("[data-vo-size]").value = String(o.size);
      $("[data-vo-status]").value = o.status;
      $("[data-vo-sort]").value = o.sort;
      $("[data-vo-text]").value = o.text;
      $("[data-vo-contrast]").checked = o.contrast;
      views();
    }

    function views() {
      list.textContent = "";
      var all = P.views();
      $("[data-vo-none]").hidden = all.length > 0;
      all.forEach(function (v) {
        var li = KW.el("li", "vo-view");
        var open = KW.el("button", "link vo-name", v.name);
        open.type = "button";
        open.title = "Show this view";
        open.addEventListener("click", function () { use(v); });
        li.appendChild(open);
        var def = KW.el("label", "vo-def");
        var cb = KW.el("input");
        cb.type = "checkbox";
        cb.checked = !!v.isDefault;
        cb.addEventListener("change", function () {
          P.setDefault(cb.checked ? v.name : "");
          paint();
          hooks.saved();
        });
        def.appendChild(cb);
        def.appendChild(document.createTextNode("Open by default"));
        li.appendChild(def);
        li.appendChild(action("Rename", function () { rename(li, v); }));
        li.appendChild(action("Delete", function () {
          P.removeView(v.name);
          paint();
          hooks.saved();
          KW.announce("Deleted the view " + v.name + ".");
        }));
        list.appendChild(li);
      });
    }

    function action(text, fn) {
      var b = KW.el("button", "link vo-act", text);
      b.type = "button";
      b.addEventListener("click", fn);
      return b;
    }

    function rename(li, v) {
      li.textContent = "";
      var input = KW.el("input", "vo-rename");
      input.value = v.name;
      input.maxLength = 40;
      input.setAttribute("aria-label", "New name for " + v.name);
      li.appendChild(input);
      li.appendChild(action("Save", done));
      li.appendChild(action("Cancel", paint));
      input.addEventListener("keydown", function (e) {
        if (e.key === "Enter") { e.preventDefault(); done(); }
        if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); paint(); }
      });
      input.focus();
      function done() {
        if (!P.renameView(v.name, input.value)) {
          input.setAttribute("aria-invalid", "true");
          return KW.announce("That name is empty or already used.");
        }
        paint();
        hooks.saved();
      }
    }

    /* use shows a saved view with its columns. */
    function use(v) {
      P.useView(v);
      box.open = false;
      hooks.open(v.query);
    }

    var sortSel = $("[data-vo-sort]");
    SORTS.forEach(function (s) { option(sortSel, s[0], s[1]); });
    [["density", "density"], ["status", "status"], ["sort", "sort"]].forEach(function (p) {
      $("[data-vo-" + p[0] + "]").addEventListener("change", function (e) {
        P.set(p[1], e.target.value || undefined);
        if (p[0] === "density") hooks.redraw();
      });
    });
    /* Text size and contrast apply at once, on every page. */
    function look() {
      P.setLook($("[data-vo-text]").value, $("[data-vo-contrast]").checked);
      hooks.redraw();
    }
    $("[data-vo-text]").addEventListener("change", look);
    $("[data-vo-contrast]").addEventListener("change", look);
    $("[data-vo-size]").addEventListener("change", function (e) {
      var size = parseInt(e.target.value, 10);
      P.set("size", size);
      hooks.open(LQ.search(Object.assign({}, hooks.view(), { pageSize: size, page: 1 })));
    });
    var saveErr = KW.el("p", "vo-err");
    saveErr.hidden = true;
    $("[data-vo-save]").after(saveErr);
    $("[data-vo-save]").addEventListener("submit", function (e) {
      e.preventDefault();
      var input = KW.$("input", e.target);
      var f = Object.assign({}, hooks.view(), { page: 1 });
      var ok = P.saveView(input.value, LQ.search(f));
      if (ok !== true) {
        input.setAttribute("aria-invalid", "true");
        saveErr.textContent = ok === "full" ?
          "Couldn't save: browser storage is full." :
          "Give the view a name (up to 20 views).";
        saveErr.hidden = false;
        return KW.announce(saveErr.textContent);
      }
      saveErr.hidden = true;
      input.removeAttribute("aria-invalid");
      KW.announce("Saved the view " + input.value.trim() + ".");
      input.value = "";
      paint();
      hooks.saved();
    });
    $("[data-vo-cols]").addEventListener("click", function () {
      box.open = false;
      hooks.columns();
    });
    $("[data-vo-reset]").addEventListener("click", function () {
      P.reset();
      paint();
      hooks.saved();
      hooks.redraw();
      KW.announce("View options are back to their defaults.");
    });
    box.addEventListener("toggle", function () { if (box.open) paint(); });
    function close(e) {
      // A button that redrew itself is no longer in the page: still inside.
      if (box.open && e.target.isConnected && !box.contains(e.target)) box.open = false;
    }
    function esc(e) {
      if (e.key === "Escape" && box.open) {
        box.open = false;
        KW.$("summary", box).focus();
      }
    }
    document.addEventListener("click", close);
    document.addEventListener("keydown", esc);
    paint();
    return { use: use, stop: function () {
      document.removeEventListener("click", close);
      document.removeEventListener("keydown", esc);
    } };
  };
})();
