/* kwatch dashboard: the preview drawer of the Issues table. A click on
   a row (or Enter on its title) opens the issue in a panel on the
   right, over the table, which keeps its filters, page, scroll and
   focused row. The panel draws the issue page's summary (incpanel.js;
   no tabs, links to them on the full page instead),
   with Previous / Next through the rows of the table (also j / k and
   [ / ]), "Open full page" and Close (×, Esc or a click beside it).
   Focus stays inside while it is open, and while it is open the page
   behind gets no keys. Under 900px wide it covers the
   screen. issues.js owns it and keeps ?open=<id> in the address. */
(function () {
  "use strict";
  var KW = window.KW;
  var MOVES = { j: 1, "]": 1, k: -1, "[": -1 };
  var FOCUSABLE = "a[href], button:not([disabled]), input, select, textarea, " +
    "summary, [tabindex]:not([tabindex='-1'])";

  function typing(t) {
    return t && (t.isContentEditable || /^(INPUT|SELECT|TEXTAREA)$/.test(t.tagName));
  }

  /* drawer builds the drawer into host. hooks: ids() is the issue IDs
     of the table rows in order, terms() the search words to mark,
     moved(id) says Previous / Next showed another issue, closed(id,
     byReader) that it closed (byReader: ×, Esc or a click beside). */
  KW.drawer = function (host, hooks) {
    host.appendChild(KW.clone("tpl-drawer"));
    var el = host.lastElementChild;
    var $ = function (sel) { return KW.$(sel, el); };
    var dialog = $("[data-drw-panel]"), body = $("[data-drw-body]");
    var prev = $("[data-drw-prev]"), next = $("[data-drw-next]");
    var pos = $("[data-drw-pos]"), full = $("[data-drw-full]");
    var current = "", panel = null;

    function notFound() {
      body.textContent = "";
      body.appendChild(KW.el("h2", "drw-gone", "We can't find that issue"));
      body.appendChild(KW.el("p", "lead-404",
        "It may have been resolved more than 7 days ago, or the link is wrong."));
    }

    /* drawn runs after every draw. A redraw can remove the focused
       node; focus then goes back to the dialog, so keys keep working. */
    function drawn(root, inc) {
      // The dialog is named after the issue once it is known.
      if (inc.title) dialog.setAttribute("aria-label", inc.title);
      var h1 = KW.$("[data-title]", root);
      KW.highlight(h1, inc.title, hooks.terms());
      KW.clampTitle(h1);
      if (current && !dialog.contains(document.activeElement)) {
        dialog.focus({ preventScroll: true });
      }
    }

    /* nav points Previous and Next at the rows around the issue. */
    function nav() {
      var ids = hooks.ids(), at = ids.indexOf(current);
      prev.disabled = at <= 0;
      next.disabled = at < 0 || at >= ids.length - 1;
      pos.textContent = at < 0 ? "" : (at + 1) + " of " + ids.length;
      if (at >= 0) pos.appendChild(KW.el("span", "drw-page", " on this page"));
    }

    function show(id) {
      if (panel) panel.stop();
      current = id;
      dialog.setAttribute("aria-label", "Issue preview");
      body.textContent = "";
      body.scrollTop = 0;
      full.setAttribute("href", "/issues/" + encodeURIComponent(id));
      panel = KW.incidentPanel(body, id, { page: false, notFound: notFound, drawn: drawn });
      nav();
      // Opened here counts as seen, as on the issue page (visits.js).
      if (KW.visits) KW.visits.opened(id);
    }

    function move(by) {
      var ids = hooks.ids(), to = ids[ids.indexOf(current) + by];
      if (ids.indexOf(current) < 0 || !to) return;
      show(to);
      hooks.moved(to);
      KW.announce("Showing issue " + (ids.indexOf(to) + 1) + " of " + ids.length);
    }

    function open(id) {
      var was = !!current;
      show(id);
      el.hidden = false;
      document.documentElement.classList.add("drw-open");
      document.addEventListener("keydown", outside, true);
      if (!was) dialog.focus({ preventScroll: true });
    }

    function close(byReader) {
      if (!current) return;
      var id = current;
      if (panel) panel.stop();
      panel = null;
      current = "";
      el.hidden = true;
      body.textContent = "";
      document.documentElement.classList.remove("drw-open");
      document.removeEventListener("keydown", outside, true);
      hooks.closed(id, byReader);
    }

    /* trap keeps Tab inside the drawer. */
    function trap(e) {
      var list = KW.$$(FOCUSABLE, dialog).filter(function (n) {
        return n.getClientRects().length > 0;
      });
      if (!list.length) return;
      var first = list[0], last = list[list.length - 1];
      if (e.shiftKey && (document.activeElement === first || document.activeElement === dialog)) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    }

    /* keys: Esc closes, j / k and [ / ] move, Tab stays inside. Keys
       the page would take ("/" search, "?" help) stop here: they would
       act behind the open preview. */
    dialog.addEventListener("keydown", function (e) {
      if (e.key === "Tab") return trap(e);
      if (e.metaKey || e.ctrlKey || e.altKey || typing(e.target)) return;
      if (e.key === "/" || e.key === "?") return e.stopPropagation();
      if (e.key === "Escape") {
        if (e.defaultPrevented) return;
        e.preventDefault();
        e.stopPropagation();
        return close(true);
      }
      if (!MOVES[e.key]) return;
      e.preventDefault();
      e.stopPropagation();
      move(MOVES[e.key]);
    });

    /* outside: a key while focus is not in the drawer (a redraw removed
       the focused node). Focus goes back to the dialog and the key acts
       on the drawer, never on the page behind it. */
    function outside(e) {
      if (!current || dialog.contains(e.target)) return;
      dialog.focus({ preventScroll: true });
      e.stopPropagation();
      if (e.key === "Tab") {
        e.preventDefault();
        return;
      }
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === "Escape") {
        e.preventDefault();
        return close(true);
      }
      if (MOVES[e.key]) {
        e.preventDefault();
        return move(MOVES[e.key]);
      }
      if (e.key === "/" || e.key === "?") e.preventDefault();
    }
    $("[data-drw-shade]").addEventListener("click", function () { close(true); });
    $("[data-drw-close]").addEventListener("click", function () { close(true); });
    prev.addEventListener("click", function () { move(-1); });
    next.addEventListener("click", function () { move(1); });

    return { open: open, close: close, nav: nav,
      id: function () { return current; },
      stop: function () {
        if (panel) panel.stop();
        document.documentElement.classList.remove("drw-open");
        document.removeEventListener("keydown", outside, true);
      } };
  };
})();
