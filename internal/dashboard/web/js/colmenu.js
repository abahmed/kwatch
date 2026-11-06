/* kwatch dashboard: the column headings of the Issues table. Each has
   a menu (its ⋮ button, or a right-click on the heading): sort
   ascending or descending, hide the column, move it left or right. Its
   right edge drags to resize the column (arrow keys on the edge do the
   same; a double-click fits it to what it shows). Columns nobody sized
   keep a fixed width, so they don't move between views; the title
   takes the room left (at least 14rem). A column hidden at this width
   (issues.css hides Where under 1100px) takes no room. issuetable.js
   builds the headings. */
(function () {
  "use strict";
  var KW = window.KW;
  var menu = null, opener = null, sized = null;
  /* Widths, in rem, of the columns nobody dragged; where takes a share of the table (at least LEAST rem). */
  var BASE = { sev: 8.5, status: 9.5, last: 6.5, started: 7, dur: 6.5,
    ack: 6.5 };
  var SHARE = { where: "24%" };
  var LEAST = { where: 10, title: 14 };
  var HEAD_MAX = 120;

  function close(refocus) {
    if (!menu) return;
    menu.remove();
    menu = null;
    document.removeEventListener("pointerdown", outside, true);
    if (opener) opener.setAttribute("aria-expanded", "false");
    if (refocus && opener && opener.isConnected) opener.focus();
  }
  function outside(e) {
    if (menu && !menu.contains(e.target) && e.target !== opener) close(false);
  }

  /* open shows the menu of a column at x, y (the page's viewport). */
  function open(button, items, x, y) {
    close(false);
    opener = button;
    menu = KW.el("ul", "colmenu");
    menu.setAttribute("role", "menu");
    menu.setAttribute("aria-label", button.getAttribute("aria-label"));
    items.forEach(function (it) {
      var li = KW.el("li", "colmenu-i", it.label);
      li.setAttribute("role", "menuitem");
      li.tabIndex = -1;
      if (it.off) li.setAttribute("aria-disabled", "true");
      li.addEventListener("click", function () {
        if (it.off) return;
        close(false);
        it.run();
      });
      menu.appendChild(li);
    });
    menu.addEventListener("keydown", function (e) {
      if (e.key === "Escape" || e.key === "Tab") {
        e.preventDefault();
        return close(true);
      }
      if ((e.key === "Enter" || e.key === " ") && e.target.getAttribute("role") === "menuitem") {
        e.preventDefault();
        e.target.click();
      }
    });
    KW.listKeys(menu, "[role=menuitem]");
    document.body.appendChild(menu);
    var w = menu.offsetWidth, h = menu.offsetHeight;
    var vw = document.documentElement.clientWidth;
    menu.style.left = Math.max(8, Math.min(x, vw - w - 8)) + "px";
    menu.style.top = Math.max(8, Math.min(y, window.innerHeight - h - 8)) + "px";
    button.setAttribute("aria-expanded", "true");
    document.addEventListener("pointerdown", outside, true);
    KW.$("[role=menuitem]:not([aria-disabled])", menu).focus();
  }
  var resizing = 0;
  window.addEventListener("resize", function () {
    close(false);
    window.clearTimeout(resizing);
    resizing = window.setTimeout(function () {
      if (sized && sized.table.isConnected) size(sized.table, sized.cols);
    }, 150);
  });
  // Capturing: the table's frame scrolls on its own on wide screens.
  document.addEventListener("scroll", function () { close(false); },
    { passive: true, capture: true });

  /* contentWidth: how wide the cells of a column want to be: the
     widest content plus the cell's own padding (and the heading's
     sort arrow). */
  function contentWidth(table, key) {
    var range = document.createRange(), max = 0;
    KW.$$("td.c-" + key + ", th.c-" + key, table).forEach(function (cell) {
      var inner = cell.tagName === "TH" ? KW.$(".it-sort, .it-th", cell) : cell;
      if (!inner) return;
      range.selectNodeContents(inner);
      var css = window.getComputedStyle(cell);
      var w = range.getBoundingClientRect().width + parseFloat(css.paddingLeft) +
        parseFloat(css.paddingRight) + parseFloat(css.borderLeftWidth) + 1;
      if (cell.hasAttribute("aria-sort")) w += 14;
      // A long heading may end in "…" (its tooltip has it whole).
      if (cell.tagName === "TH") w = Math.min(w, HEAD_MAX);
      max = Math.max(max, w);
    });
    return Math.ceil(max);
  }

  /* size gives each heading its width: dragged or its base; the
     table is kept at least as wide as the shown ones need. */
  function size(table, cols) {
    sized = { table: table, cols: cols };
    var widths = KW.prefs.widths(), sum = 0;
    var rem = parseFloat(window.getComputedStyle(document.documentElement).fontSize) || 16;
    cols.forEach(function (key) {
      var th = KW.$("th.c-" + key, table);
      var w = widths[key] || (BASE[key] || 0) * rem;
      th.style.width = widths[key] ? w + "px" : BASE[key] ? BASE[key] + "rem" : SHARE[key] || "";
      if (window.getComputedStyle(th).display === "none") return;
      sum += w || (LEAST[key] || 0) * rem;
    });
    table.style.minWidth = sum + "px";
    var wrap = table.parentNode;
    wrap.classList.toggle("scrollx", sum > wrap.clientWidth + 1);
  }

  /* resizer: the draggable right edge of th. */
  function resizer(th, key, table, cols, label) {
    var edge = KW.el("span", "it-rs");
    edge.tabIndex = 0;
    edge.setAttribute("role", "separator");
    edge.setAttribute("aria-orientation", "vertical");
    edge.setAttribute("aria-label", "Resize " + label + " (arrow keys; double-click fits)");
    edge.title = "Drag to resize. Double-click to fit.";
    function set(w) {
      KW.prefs.setWidth(key, w);
      size(table, cols);
    }
    edge.addEventListener("pointerdown", function (e) {
      e.preventDefault();
      e.stopPropagation();
      var startX = e.clientX, startW = th.getBoundingClientRect().width;
      edge.setPointerCapture(e.pointerId);
      th.classList.add("is-resizing");
      function move(ev) {
        var w = Math.max(KW.prefs.MIN_W, Math.min(KW.prefs.MAX_W, startW + ev.clientX - startX));
        th.style.width = w + "px";
      }
      /* Release, cancel and lost capture all end the drag, once. */
      function up(ev) {
        edge.removeEventListener("pointermove", move);
        edge.removeEventListener("pointerup", up);
        edge.removeEventListener("pointercancel", up);
        edge.removeEventListener("lostpointercapture", up);
        th.classList.remove("is-resizing");
        set(startW + ev.clientX - startX);
      }
      edge.addEventListener("pointermove", move);
      edge.addEventListener("pointerup", up);
      edge.addEventListener("pointercancel", up);
      edge.addEventListener("lostpointercapture", up);
    });
    edge.addEventListener("click", function (e) { e.stopPropagation(); });
    edge.addEventListener("dblclick", function (e) {
      e.stopPropagation();
      set(contentWidth(table, key));
      KW.announce(label + " fits its content.");
    });
    edge.addEventListener("keydown", function (e) {
      var by = { ArrowLeft: -16, ArrowRight: 16 }[e.key];
      if (!by) return;
      e.preventDefault();
      set(th.getBoundingClientRect().width + by);
    });
    th.appendChild(edge);
  }

  /* head adds the menu button and the resize edge to th. hooks:
     sort(field, dir), hide(key), move(key, by). */
  function head(th, c, table, cols, hooks) {
    var at = cols.indexOf(c.key);
    var b = KW.el("button", "it-menu");
    b.type = "button";
    b.appendChild(KW.icon("more"));
    b.setAttribute("aria-label", c.label + " column menu");
    b.setAttribute("aria-haspopup", "menu");
    b.setAttribute("aria-expanded", "false");
    b.title = "Sort, hide or move this column";
    b.dataset.fk = "menu:" + c.key;
    var words = KW.issueTable.SORTS[c.sort] || ["Sort ascending", "Sort descending"];
    var items = [
      { label: words[0], off: !c.sort, run: function () { hooks.sort(c.sort, "asc"); } },
      { label: words[1], off: !c.sort, run: function () { hooks.sort(c.sort, "desc"); } },
      { label: "Hide column", off: !!c.always, run: function () { hooks.hide(c.key); } },
      { label: "Move left", off: at <= 1, run: function () { hooks.move(c.key, -1); } },
      { label: "Move right", off: at < 1 || at === cols.length - 1,
        run: function () { hooks.move(c.key, 1); } }
    ];
    b.addEventListener("click", function (e) {
      e.stopPropagation();
      if (menu && opener === b) return close(true);
      var r = b.getBoundingClientRect();
      open(b, items, r.left, r.bottom + 4);
    });
    th.addEventListener("contextmenu", function (e) {
      e.preventDefault();
      open(b, items, e.clientX, e.clientY);
    });
    th.appendChild(b);
    resizer(th, c.key, table, cols, c.label);
  }

  KW.columnHead = { head: head, size: size, close: close,
    isOpen: function () { return !!menu; } };
})();
