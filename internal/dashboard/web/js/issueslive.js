/* kwatch dashboard: live updates of the Issues table without rows
   jumping while someone reads. A refresh that only changes rows already
   shown updates them in place. New rows, rows that left and a new order
   are shown at once only when the reader is at the top of the first
   page, has no row focused and hasn't touched the page for 10 s;
   otherwise a bar at the bottom of the screen says what changed
   ("2 new urgent · 12 changed · Show"). */
(function () {
  "use strict";
  var KW = window.KW;
  var QUIET = 10000;

  /* pageIds: the issue IDs a list response shows, in order. */
  function pageIds(data) {
    return (data.incidents || []).map(function (i) { return i.id; });
  }

  /* diff compares the rows shown with a fresh response: fresh are
     rows not shown yet, gone rows no longer in it, moved rows shown in
     another place. count is how many the bar reports. */
  function diff(shown, data) {
    var next = pageIds(data), at = {}, was = {};
    next.forEach(function (id, i) { at[id] = i; });
    shown.forEach(function (id, i) { was[id] = i; });
    var fresh = next.filter(function (id) { return was[id] === undefined; });
    var gone = {};
    shown.forEach(function (id) { if (at[id] === undefined) gone[id] = true; });
    var kept = next.filter(function (id) { return was[id] !== undefined; });
    var order = shown.filter(function (id) { return at[id] !== undefined; });
    var moved = kept.filter(function (id, i) { return order[i] !== id; }).length;
    var goneN = Object.keys(gone).length;
    return { fresh: fresh, gone: gone, moved: moved,
      count: fresh.length + goneN + (fresh.length || goneN ? 0 : moved),
      same: !fresh.length && !goneN && !moved };
  }

  /* byId: the issues of a response by ID. */
  function byId(data) {
    var out = {};
    (data.incidents || []).forEach(function (i) { out[i.id] = i; });
    return out;
  }

  KW.liveLogic = { pageIds: pageIds, diff: diff, byId: byId };

  /* liveControl wires the bar ([data-livebar]) of root. hooks: show()
     applies what the bar offers. */
  KW.liveControl = function (root, hooks) {
    var bar = KW.$("[data-livebar]", root);
    var touched = 0;
    var EVENTS = ["pointerdown", "keydown", "wheel", "touchstart"];
    function touch() { touched = Date.now(); }
    EVENTS.forEach(function (ev) {
      root.addEventListener(ev, touch, { passive: true });
    });
    // Capturing: the table's frame scrolls on its own on wide screens.
    document.addEventListener("scroll", touch, { passive: true, capture: true });

    /* words: "2 new urgent · 3 new · 12 changed" for a diff d of the
       rows shown and the issues of the fresh list by ID. */
    function words(d, byId) {
      var urgent = d.fresh.filter(function (id) {
        return byId[id] && byId[id].severity === "page";
      }).length;
      var parts = [];
      if (urgent) parts.push(KW.num(urgent) + " new urgent");
      if (d.fresh.length > urgent) parts.push(KW.num(d.fresh.length - urgent) + " new");
      var changed = Object.keys(d.gone).length + d.moved;
      if (changed) parts.push(KW.num(changed) + " changed");
      return parts.join(" · ");
    }

    KW.$("[data-livebar-show]", bar).addEventListener("click", function () {
      bar.hidden = true;
      hooks.show();
    });

    return {
      /* calm: a fresh order may replace the rows now. */
      calm: function (page, table) {
        var focused = document.activeElement;
        return page === 1 && KW.scrollTop() < 80 &&
          !(focused && table.contains(focused)) &&
          Date.now() - touched > QUIET;
      },
      offer: function (d, byId) {
        var text = words(d, byId);
        bar.hidden = !text;
        KW.$("[data-livebar-text]", bar).textContent = text;
      },
      clear: function () { bar.hidden = true; },
      stop: function () {
        EVENTS.forEach(function (ev) { root.removeEventListener(ev, touch, { passive: true }); });
        document.removeEventListener("scroll", touch, { passive: true, capture: true });
      }
    };
  };
})();
