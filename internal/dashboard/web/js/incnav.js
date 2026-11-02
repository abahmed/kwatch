/* kwatch dashboard: "← Previous · 3 of 4,812 · Next →" on an issue page.
   When the Issues table draws, it remembers its address, its page
   (number, size, how many pages there are) and the issues in the
   order shown (sessionStorage key kwatch.listnav, this tab only). An
   issue opened from it can walk that order with the bar above the
   issue, or with [ and ] (j and k too), across pages: at the edge of
   the page the next or previous page is read from the API. "Back"
   returns to the exact table, filters, sort and page. Without storage
   it works until the page is reloaded; an issue not in the list shows
   no bar. */
(function () {
  "use strict";
  var KW = window.KW;
  var KEY = "kwatch.listnav";
  var MAX = 500;
  var memory = null;
  /* turning counts page turns: only the latest one may open an issue,
     and only while the reader is still on the issue it started from. */
  var turning = 0;

  /* ---- plain data ---- */

  /* position: where id is in list, as {at, total, prev, next}. prev and
     next are an ID, "<" / ">" for the edge of the page when another
     page is there, or "" at the ends. null when id isn't listed. */
  function position(list, id) {
    var ids = list && list.ids;
    var i = Array.isArray(ids) ? ids.indexOf(id) : -1;
    if (i < 0) return null;
    var prev = ids[i - 1] || (list.page > 1 ? "<" : "");
    var next = ids[i + 1] || (list.page < list.pages ? ">" : "");
    if (!prev && !next) return null;
    return { at: (list.page - 1) * list.pageSize + i + 1, total: list.total || ids.length,
      prev: prev, next: next };
  }

  /* valid: a remembered list is the Issues table at a local path. */
  function valid(v) {
    return !!v && typeof v.url === "string" && /^\/issues(\?[^#]*)?$/.test(v.url) &&
      typeof v.page === "number" && typeof v.pageSize === "number" &&
      typeof v.pages === "number" && Array.isArray(v.ids) &&
      v.ids.every(function (id) { return typeof id === "string"; });
  }

  /* ---- storage ---- */

  function remember(list) {
    memory = { url: list.url, ids: list.ids.slice(0, MAX), page: list.page,
      pageSize: list.pageSize, pages: list.pages, total: list.total || 0 };
    try {
      sessionStorage.setItem(KEY, JSON.stringify(memory));
    } catch (e) { /* blocked storage: this page only */ }
  }

  function recall() {
    try {
      var v = JSON.parse(sessionStorage.getItem(KEY) || "null");
      if (valid(v)) return v;
    } catch (e) { /* blocked or damaged: fall back to memory */ }
    return valid(memory) ? memory : null;
  }

  /* turn reads the page before ("<") or after (">") the remembered one
     and opens its last or first issue. */
  function turn(list, edge) {
    var LQ = KW.listQuery, mine = ++turning, from = location.pathname;
    var f = LQ.parse(list.url.replace(/^\/issues/, ""));
    f.page = Math.max(1, list.page + (edge === ">" ? 1 : -1));
    return KW.api("/api/incidents?" + LQ.api(f, KW.now(), KW.utc).toString())
      .then(function (res) {
        if (mine !== turning || location.pathname !== from) return;
        // A failed read (a 401 included: the next page load signs in
        // again) leaves the reader where they are.
        if (res.status !== 200 || !res.data) return;
        var d = res.data, ids = (d.incidents || []).map(function (i) { return i.id; });
        if (!ids.length) return;
        remember({ url: "/issues" + LQ.search(f), ids: ids, page: f.page,
          pageSize: f.pageSize, pages: d.totalPages || 0, total: d.total });
        KW.go("/issues/" + encodeURIComponent(edge === ">" ? ids[0] : ids[ids.length - 1]));
      });
  }

  /* ---- the bar ---- */

  function point(a, to) {
    a.dataset.edge = to === "<" || to === ">" ? to : "";
    if (to && !a.dataset.edge) a.setAttribute("href", "/issues/" + encodeURIComponent(to));
    else if (to) a.setAttribute("href", "#");
    if (to) a.removeAttribute("aria-disabled");
    else {
      a.removeAttribute("href");
      a.setAttribute("aria-disabled", "true");
    }
  }

  function bar() { return document.querySelector("[data-incnav]"); }

  /* show fills the bar for issue id, or hides it. It also points
     "Back" (KW.lastList) at the exact table the issue was opened from. */
  function show(id) {
    var b = bar();
    if (!b) return;
    var list = recall();
    if (list) KW.lastList = list.url;
    var pos = list && position(list, id);
    b.hidden = !pos;
    if (!pos) return;
    point(b.querySelector("[data-incnav-prev]"), pos.prev);
    point(b.querySelector("[data-incnav-next]"), pos.next);
    var where = b.querySelector("[data-incnav-pos]");
    where.textContent = KW.num(pos.at) + " of " + KW.num(pos.total);
    where.title = "In your Issues list";
  }

  /* An edge link reads the next page before it opens an issue. A click
     with Cmd, Ctrl, Shift or Alt, or another button, is the browser's. */
  document.addEventListener("click", function (e) {
    var a = e.target.closest && e.target.closest("[data-incnav] a[data-edge]");
    if (!a || !a.dataset.edge || e.button !== 0 || e.metaKey || e.ctrlKey ||
        e.shiftKey || e.altKey) return;
    e.preventDefault();
    e.stopPropagation();
    var list = recall();
    if (list) turn(list, a.dataset.edge);
  }, true);

  /* The issue page shows the bar while it is open. */
  var detailView = KW.detailView;
  KW.detailView = function (main, r) {
    show(r.id);
    var view = detailView(main, r);
    return { stop: function () {
      var b = bar();
      if (b) b.hidden = true;
      if (view && view.stop) view.stop();
    } };
  };

  /* Keys: [ and ] (and j / k) walk the list on an issue page. */
  var KEYS = { "[": "prev", "]": "next", k: "prev", j: "next" };
  document.addEventListener("keydown", function (e) {
    var t = e.target, b = bar();
    if (!KEYS[e.key] || e.metaKey || e.ctrlKey || e.altKey || !b || b.hidden) return;
    if (t && (t.isContentEditable || /^(INPUT|SELECT|TEXTAREA)$/.test(t.tagName))) return;
    var a = b.querySelector("[data-incnav-" + KEYS[e.key] + "]");
    if (!a.getAttribute("href")) return;
    e.preventDefault();
    a.click();
  });

  KW.incNav = { remember: remember, recall: recall };
})();
