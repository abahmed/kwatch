/* kwatch dashboard: the "Columns" panel above the Issues table. Every
   column with a checkbox to show it, a handle to drag it to another
   place (and Move up / Move down buttons for the keyboard), the
   presets Compact, Standard and Everything, and "Reset columns".
   Severity and Issue are always shown; severity stays first. Kept in
   this browser with the view options (prefs.js). */
(function () {
  "use strict";
  var KW = window.KW;

  /* columnsPanel wires wrap ([data-cols]: its button and panel).
     redraw() draws the table again after a change. */
  KW.columnsPanel = function (wrap, redraw) {
    var P = KW.prefs;
    var trigger = KW.$("[data-cols-btn]", wrap);
    var pop = KW.$("[data-cols-pop]", wrap);
    var list = KW.$("[data-cols-list]", pop);
    var presets = KW.$("[data-cols-presets]", pop);
    var dragging = null;
    var pv = KW.popover(wrap, trigger, pop, {
      opened: function () {
        paint();
        var first = KW.$("button, input", pop);
        if (first) first.focus();
      }
    });

    function same(a, b) { return a.join() === b.join(); }

    /* apply stores visible keys in order and redraws; focus goes back
       to the same control of the same column. */
    function apply(visible, order, focusSel) {
      P.setColumns(visible, order);
      redraw();
      paint();
      if (focusSel) {
        var again = KW.$(focusSel, list);
        if (again && !again.disabled) again.focus();
      }
    }

    function move(key, by) {
      var order = P.order(), at = order.indexOf(key), to = at + by;
      if (at < 1 || to < 1 || to >= order.length) return;
      order.splice(at, 1);
      order.splice(to, 0, key);
      apply(P.columns(), order, '[data-key="' + key + '"] [data-move="' +
        (by < 0 ? "up" : "down") + '"]');
      KW.announce(P.column(key).label + " moved " + (by < 0 ? "up" : "down") + ".");
    }

    function button(cls, icon, label, disabled) {
      var b = KW.el("button", "cols-mv " + cls);
      b.type = "button";
      b.appendChild(KW.icon(icon));
      b.setAttribute("aria-label", label);
      b.title = label;
      b.disabled = disabled;
      return b;
    }

    function row(key, i, order, shown) {
      var c = P.column(key);
      var li = KW.el("li", "cols-row");
      li.dataset.key = key;
      var pinned = key === "sev";
      li.draggable = !pinned;
      var grip = KW.el("span", "cols-grip" + (pinned ? " is-off" : ""));
      grip.appendChild(KW.icon("grip"));
      grip.title = pinned ? "Severity stays first" : "Drag to move";
      li.appendChild(grip);
      var l = KW.el("label", "cols-c");
      var cb = KW.el("input");
      cb.type = "checkbox";
      cb.checked = shown.indexOf(key) >= 0;
      cb.disabled = !!c.always;
      cb.addEventListener("change", function () {
        var keys = P.columns().filter(function (k) { return k !== key; });
        if (cb.checked) keys.push(key);
        apply(keys, null, '[data-key="' + key + '"] input');
      });
      l.appendChild(cb);
      l.appendChild(document.createTextNode(c.label));
      if (c.always) l.appendChild(KW.el("span", "cols-always", "always"));
      li.appendChild(l);
      var up = button("", "up", "Move " + c.label + " up", pinned || i <= 1);
      up.dataset.move = "up";
      up.addEventListener("click", function () { move(key, -1); });
      var down = button("", "down", "Move " + c.label + " down",
        pinned || i === order.length - 1);
      down.dataset.move = "down";
      down.addEventListener("click", function () { move(key, 1); });
      li.appendChild(up);
      li.appendChild(down);
      return li;
    }

    function paint() {
      var order = P.order(), shown = P.columns();
      list.textContent = "";
      order.forEach(function (key, i) { list.appendChild(row(key, i, order, shown)); });
      KW.$$("[data-preset]", presets).forEach(function (b) {
        var p = P.PRESETS.filter(function (x) { return x.id === b.dataset.preset; })[0];
        var want = order.filter(function (k) { return p.cols.indexOf(k) >= 0; });
        b.setAttribute("aria-pressed", same(want, shown) ? "true" : "false");
      });
    }

    /* Dragging a row drops it before the row under the pointer. */
    list.addEventListener("dragstart", function (e) {
      var li = e.target.closest && e.target.closest("[data-key]");
      if (!li || !li.draggable) return;
      dragging = li.dataset.key;
      li.classList.add("is-dragging");
      e.dataTransfer.effectAllowed = "move";
      e.dataTransfer.setData("text/plain", dragging);
    });
    list.addEventListener("dragover", function (e) {
      var li = e.target.closest && e.target.closest("[data-key]");
      if (!dragging || !li || li.dataset.key === "sev") return;
      e.preventDefault();
      KW.$$(".is-drop", list).forEach(function (x) { x.classList.remove("is-drop"); });
      li.classList.add("is-drop");
    });
    list.addEventListener("dragend", function () {
      dragging = null;
      KW.$$(".is-drop, .is-dragging", list).forEach(function (x) {
        x.classList.remove("is-drop", "is-dragging");
      });
    });
    list.addEventListener("drop", function (e) {
      var li = e.target.closest && e.target.closest("[data-key]");
      if (!dragging || !li) return;
      e.preventDefault();
      var order = P.order().filter(function (k) { return k !== dragging; });
      order.splice(Math.max(1, order.indexOf(li.dataset.key)), 0, dragging);
      var key = dragging;
      dragging = null;
      apply(P.columns(), order);
      KW.announce(P.column(key).label + " moved.");
    });

    P.PRESETS.forEach(function (p) {
      var b = KW.el("button", "qv", p.label);
      b.type = "button";
      b.dataset.preset = p.id;
      b.title = p.cols.map(function (k) { return P.column(k).label; }).join(", ");
      b.addEventListener("click", function () {
        apply(p.cols, null, '[data-preset="' + p.id + '"]');
        KW.$('[data-preset="' + p.id + '"]', presets).focus();
        KW.announce("Columns: " + p.label + ".");
      });
      presets.appendChild(b);
    });
    KW.$("[data-cols-reset]", pop).addEventListener("click", function () {
      P.resetColumns();
      redraw();
      paint();
      KW.announce("Columns are back to the standard set.");
    });

    return { open: function () { pv.open(); trigger.scrollIntoView({ block: "nearest" }); },
      paint: paint };
  };
})();
