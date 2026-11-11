/* kwatch dashboard: how an open issue takes a newer reading without
   disturbing the reader (incpanel.js calls it).
   - It updates in place, on its own, as the rest of the dashboard does.
   - It waits while the reader is selecting text in the issue or has a
     control in it focused from the keyboard (a search box, a menu, a
     button reached with Tab): a quiet note says an update waits, and it
     applies as soon as the selection or the focus goes (or at "Show
     now").
   - The open tab, the scroll position (the part at the top of the
     window stays where it is) and the open folds are kept.
   - What changed gets a short highlight, and the head of the summary
     says "Updated just now" for a few seconds; screen readers hear
     "This issue was updated." */
(function () {
  "use strict";
  var KW = window.KW;
  var PARTS = ".ds-head, [data-ds-row], .ds-notes, [data-fold], [data-tab]";

  /* KW.issueBusy: the reader is selecting text in root or has a control
     in it focused from the keyboard. A focus left by a mouse click
     (not :focus-visible) does not count, nor the tab bar, which an
     update never redraws. */
  KW.issueBusy = function (root) {
    if (KW.selecting(root)) return true;
    var a = document.activeElement;
    if (!a || a === document.body || !root.contains(a)) return false;
    if (a.closest("[data-tabs]") || a.getAttribute("role") === "tabpanel") return false;
    var typing = /^(INPUT|SELECT|TEXTAREA)$/.test(a.tagName) || a.isContentEditable;
    var ring = false;
    try { ring = a.matches(":focus-visible"); } catch (e) { ring = false; }
    return typing || ring;
  };

  function shown(el) { return !el.hidden && el.getClientRects().length > 0; }

  /* key: a name for a fold or a part that survives a redraw. */
  function key(el) {
    var fold = el.closest("[data-fold]"), sum = el.querySelector(":scope > summary");
    return (fold ? fold.dataset.fold : "") + "|" + (sum ? sum.textContent.trim() : "");
  }
  function partKey(el, i) {
    return el.dataset.fold || el.dataset.dsRow || el.dataset.tab || el.className + i;
  }

  /* scroller: the drawer's body when the issue is in the drawer, else
     the page. */
  function scroller(root) {
    var box = root.closest(".drw-body");
    return box || null;
  }

  /* KW.keepPlace notes what the reader sees (the open folds, the part
     at the top of the window, the text of each part) and returns a
     function that, after a redraw, puts it back and marks what
     changed. */
  KW.keepPlace = function (root) {
    var open = KW.$$("details[open]", root).map(key);
    var texts = {}, anchor = null, top = 0;
    var edge = scroller(root) ? scroller(root).getBoundingClientRect().top : 0;
    KW.$$(PARTS, root).forEach(function (el, i) {
      texts[partKey(el, i)] = el.textContent;
      if (anchor || !shown(el)) return;
      var r = el.getBoundingClientRect();
      if (r.bottom > edge && !el.closest("[data-tabs]")) { anchor = el; top = r.top; }
    });
    return function () {
      KW.$$("details:not([open])", root).forEach(function (d) {
        if (open.indexOf(key(d)) >= 0) d.open = true;
      });
      if (anchor && anchor.isConnected && shown(anchor)) {
        var dy = anchor.getBoundingClientRect().top - top, box = scroller(root);
        if (Math.abs(dy) >= 1) {
          if (box) box.scrollTop += dy; else window.scrollBy(0, dy);
        }
      }
      var changed = KW.$$(PARTS, root).filter(function (el, i) {
        var was = texts[partKey(el, i)];
        return shown(el) && was !== undefined && was !== el.textContent;
      });
      mark(root, changed);
    };
  };

  /* mark flashes the parts that changed and says "Updated just now". */
  function mark(root, changed) {
    changed.forEach(function (el) {
      el.classList.remove("kw-changed");
      void el.offsetWidth;
      el.classList.add("kw-changed");
      window.setTimeout(function () { el.classList.remove("kw-changed"); }, 2600);
    });
    var head = KW.$(".ds-top", root);
    if (!head) return;
    var note = KW.$(".d-upd", head);
    if (!note) {
      note = KW.el("span", "d-upd", "Updated just now");
      note.setAttribute("aria-hidden", "true");
      head.insertBefore(note, KW.$(".d-acts", head));
    }
    note.hidden = false;
    window.clearTimeout(note._t);
    note._t = window.setTimeout(function () { note.hidden = true; }, 8000);
    if (KW.announce) KW.announce("This issue was updated.");
  }

  /* KW.waitNote is the quiet note shown while an update waits, with
     "Show now"; now() applies it. */
  KW.waitNote = function (now) {
    var bar = KW.el("p", "d-wait");
    bar.setAttribute("role", "status");
    bar.hidden = true;
    bar.appendChild(KW.el("span", "", "This issue changed. It updates when you finish."));
    var b = bar.appendChild(KW.el("button", "link", "Show now"));
    b.type = "button";
    b.addEventListener("click", now);
    return bar;
  };
})();
