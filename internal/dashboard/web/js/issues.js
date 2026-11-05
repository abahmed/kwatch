/* kwatch dashboard: the Issues page. One table for active and resolved
   issues; the view (status, filters, sort, page) lives in the address
   (listquery.js) and the server filters, sorts and pages it, so it
   works the same with 10 issues or 10,000. It polls
   with the list's ETag, so an unchanged list costs a 304, and applies
   changes without moving rows under the reader (issueslive.js). */
(function () {
  "use strict";
  var KW = window.KW;
  var EVERY = 20000;
  var LQ = KW.listQuery;

  KW.issuesView = function (main) {
    main.appendChild(KW.clone("tpl-issues"));
    main.classList.add("wide");
    KW.title("Issues");
    // A bare /issues opens this browser's default view; a link with a
    // view in it always wins.
    var f = location.search ? LQ.parse(location.search) : KW.prefs.start();
    var $ = function (sel) { return KW.$(sel, main); };
    var tableEl = $("[data-table]"), alertBox = $("[data-alert]");
    var empty = $("[data-empty]"), footer = $("[data-updated]");
    var sub = $("[data-sub]"), wrap = $("[data-tablewrap]");
    var last = null, pending = null, shown = [], lastOk = 0, failing = false;
    var attempt = 0, seq = 0, stopped = false, ctl = null, busy = false;
    var etag = "", etagQuery = "", lastStatus = null, slow = 0, tried = "";
    // The issue open beside the table (?open=), and the row last
    // focused or opened, for coming back to the same place.
    var openId = LQ.ID.test(new URLSearchParams(location.search).get("open") || "") ?
      new URLSearchParams(location.search).get("open") : "";
    var pushedOpen = false, lastRow = openId;
    var keep = KW.keepFocus(main);
    var y = KW.takeScroll();
    var place = y ? { y: y, id: "" } : KW.place.take(listURL());

    var filters = KW.issueFilters($("[data-filterbox]"), function (next, typing) {
      setView(next, typing);
    });
    var table = KW.issueTable(tableEl, {
      sort: function (field, dir) {
        var desc = f.dir ? f.dir === "desc" : !!KW.issueTable.DESCENDS[field];
        if (dir) setView(LQ.copy(f, { sort: field, dir: dir, page: 1 }));
        else if (f.sort === field) setView(LQ.copy(f, { dir: desc ? "asc" : "desc", page: 1 }));
        else setView(LQ.copy(f, { sort: field, dir: "", page: 1 }));
      },
      columns: function (focusKey) {
        redraw();
        cols.paint();
        var again = focusKey && KW.$("th.c-" + focusKey + " .it-menu", tableEl);
        if (again) again.focus();
      },
      open: function (id) { openIssue(id, true); }
    });
    var drawer = KW.drawer(main, {
      ids: function () { return KW.$$("tr[data-id]", tableEl).map(function (tr) { return tr.dataset.id; }); },
      terms: function () { return KW.searchTerms(f.q); },
      moved: function (id) {
        openId = lastRow = id;
        writeURL(true);
      },
      closed: function (id, byReader) {
        openId = "";
        if (byReader && pushedOpen) history.back();
        else writeURL(true);
        pushedOpen = false;
        focusRow(id);
      }
    });
    tableEl.addEventListener("focusin", function (e) {
      var tr = e.target.closest("tr[data-id]");
      if (tr) lastRow = tr.dataset.id;
    });

    /* openIssue shows id beside the table; push adds a history entry,
       so Back closes it. */
    function openIssue(id, push) {
      openId = lastRow = id;
      KW.visits.opened(id);
      drawer.open(id);
      if (push && !pushedOpen) {
        pushedOpen = true;
        writeURL(false);
      } else {
        writeURL(true);
      }
    }

    /* focusRow puts the focus back on the title of id's row. */
    function focusRow(id) {
      var link = id && KW.$('tr[data-id="' + CSS.escape(id) + '"] [data-rowlink]', tableEl);
      if (!link) return;
      link.focus({ preventScroll: true });
      link.scrollIntoView({ block: "nearest" });
    }

    function listURL() {
      var params = new URLSearchParams(location.search);
      params.delete("open");
      var rest = params.toString();
      return "/issues" + (rest ? "?" + rest : "");
    }
    function redraw() { if (last) apply(last); }
    var cols = KW.columnsPanel($("[data-cols]"), redraw);
    var quick = KW.quickViews($("[data-qv]"), function (next) { setView(next); });
    var live = KW.liveControl(main, {
      show: function () { if (pending) apply(pending); KW.scrollTo(0); }
    });
    var opts = KW.viewOptions($("[data-vopts]"), {
      view: function () { return f; },
      redraw: redraw,
      open: function (query) { setView(LQ.parse(query)); },
      saved: function () { quick.paint(f); },
      columns: function () { cols.open(); }
    });

    /* writeURL puts the view in the address: a new history entry for a
       click (so Back undoes it), the same entry while typing. */
    function writeURL(replace) {
      var list = "/issues" + LQ.search(f);
      var url = list + (openId ? "&open=" + encodeURIComponent(openId) : "");
      if (url !== location.pathname + location.search) {
        if (replace) history.replaceState(null, "", url);
        else history.pushState(null, "", url);
      }
      KW.rendered = url;
      KW.lastList = list;
    }

    function setView(next, typing) {
      f = next;
      filters.show(f);
      quick.paint(f);
      writeURL(typing);
      etag = "";
      load(true);
    }

    /* ---- loading ---- */
    function query() { return LQ.api(f, KW.now(), KW.utc).toString(); }

    function load(user) {
      if (!user && busy) return Promise.resolve(0);
      if (ctl) ctl.abort();
      ctl = window.AbortController ? new AbortController() : null;
      var mine = ++seq, q = query();
      tried = q;
      busy = true;
      window.clearTimeout(slow);
      if (!last) slow = window.setTimeout(function () { wrap.classList.add("loading"); }, 300);
      var calls = [KW.api("/api/incidents?" + q, { signal: ctl && ctl.signal,
        etag: !user && q === etagQuery ? etag : "" })];
      // The badge counts the whole active list: a filtered or other
      // list reads it on its own (an unchanged one costs a 304).
      calls.push(f.status !== "active" || LQ.used(f) ? KW.refreshBadge() : null);
      return Promise.all(calls).then(function (out) {
        if (mine === seq) busy = false;
        if (stopped || mine !== seq) return 0;
        window.clearTimeout(slow);
        wrap.classList.remove("loading");
        var res = out[0];
        if (KW.authFailed(res)) return 0;
        if (res.status === 304 && last) return fresh();
        if (res.status !== 200 || !res.data) return showFailure(res);
        etag = res.etag;
        etagQuery = q;
        receive(res.data, user);
        return 0;
      });
    }

    function fresh() {
      tableEl.classList.remove("stale");
      unrefuse();
      alertBox.textContent = "";
      lastOk = Date.now();
      failing = false;
      attempt = 0;
      KW.footer(footer, lastOk, false);
      return 0;
    }

    /* receive takes a fresh response: counts at once, rows by the rules
       of issueslive.js. */
    function receive(data, user) {
      fresh();
      header(data);
      var still = drawer.id() || KW.selecting(tableEl);
      if (user || !last || (!still && !KW.columnHead.isOpen() &&
          live.calm(f.page, tableEl))) return apply(data);
      var d = KW.liveLogic.diff(shown, data);
      // Selected text stays as it is: the rows update after.
      if (!KW.selecting(tableEl)) table.patch(KW.liveLogic.byId(data), d.gone, observe(data));
      if (d.same) {
        last = data;
        pending = null;
        return live.clear();
      }
      pending = data;
      live.offer(d, KW.liveLogic.byId(data));
    }

    /* observe: the New / Changed marks of the issues shown (visits.js);
       resolved issues are never marked. */
    function observe(data) {
      var items = (data.incidents || []).filter(function (i) { return i.state !== "resolved"; });
      return items.length || f.status !== "resolved" ? KW.visits.observe(items) : {};
    }

    /* header: the counts, the pager, the badge and the tab title. The
       badge counts the whole active list: an unfiltered Active view
       updates it from its own counts, any other view read it with
       KW.refreshBadge. */
    function header(data) {
      data.facets = data.facets || {};
      data.total = data.total || 0;
      filters.paint(data);
      if (f.status === "active" && !LQ.used(f) && data.facets.severity) {
        KW.updateBadge(data.facets.severity);
      }
      var active = KW.activeTotal || 0;
      KW.setTitle(active ? "(" + KW.num(active) + ") Issues · kwatch" : "Issues · kwatch");
      sub.textContent = KW.listNotes.subtitle(f, data);
      pager.paint(data, f.page, f.pageSize);
      // A chip removed hands the focus to the one before it, else to
      // the search box; a pager button gone to the current page.
      keep.restore(function (key) {
        return /^chip:/.test(key) ? $('[data-f="q"]') : $("[data-pager] [aria-current]");
      });
    }

    /* apply draws a response, replacing the rows. */
    function apply(data) {
      pending = null;
      live.clear();
      last = data;
      var marks = observe(data);
      quick.news(KW.visits.summary(marks), function () {
        KW.visits.markAll(data.incidents || []);
        if (last) apply(last);
      });
      var s = KW.searchQuery.parse(f.q), now = KW.now();
      var page = Object.assign({}, data, { incidents: (data.incidents || []).filter(function (i) {
        return KW.searchQuery.keep(i, s, marks, now);
      }) });
      var o = KW.prefs.options();
      var ids = table.draw(page, f, { cols: o.cols, density: o.density, marks: marks,
        terms: KW.searchTerms(f.q) });
      drawer.nav();
      shown = KW.liveLogic.pageIds(data);
      KW.incNav.remember({ url: "/issues" + LQ.search(f), ids: ids, page: f.page,
        pageSize: f.pageSize, pages: data.totalPages || 0, total: data.total });
      keep.restore();
      $("[data-pagenote]").hidden = !(s.is.new !== undefined || s.is.changed !== undefined);
      emptyState(page, ids);
      if (place) {
        if (!drawer.id()) focusRow(place.id);
        KW.scrollTo(place.y);
        place = null;
      }
    }

    /* cleared: the view without its filters; the status, order and
       page size stay. */
    function cleared() {
      return LQ.copy(LQ.blank(), { status: f.status, sort: f.sort, dir: f.dir,
        pageSize: f.pageSize });
    }

    function emptyState(page, ids) {
      empty.textContent = "";
      wrap.hidden = ids.length === 0;
      if (!wrap.hidden) return;
      if (LQ.used(f) || f.page > 1) {
        var later = f.page > 1;
        var p = KW.el("p", "nomatch", later ? "Nothing on this page. " :
          "No issues match these filters. ");
        var b = KW.el("button", "link", later ? "Go to the first page" : "Clear filters");
        b.type = "button";
        b.addEventListener("click", function () {
          if (later) setView(Object.assign({}, f, { page: 1 }));
          else setView(cleared());
        });
        p.appendChild(b);
        return empty.appendChild(p);
      }
      if (f.status === "resolved") return empty.appendChild(KW.resolvedEmpty());
      var resolved = ((page.facets || {}).state || {}).resolved || 0;
      KW.api("/api/status").then(function (res) {
        if (stopped || !wrap.hidden) return;
        lastStatus = res.status === 200 ? res.data : lastStatus;
        empty.textContent = "";
        empty.appendChild(KW.activeEmpty(lastStatus, resolved));
      });
    }

    var NEVER = 2147000000;
    var refused = null, pagerBox = $("[data-pager]");

    /* badRequest: the server refused the search or a filter (400); asking
       again would get the same answer, so nothing is retried. */
    function badRequest(res) {
      alertBox.textContent = "";
      alertBox.appendChild(KW.listNotes.badRequest(res, function () { setView(cleared()); }));
      // The rows and pager are of another view: hide them, so they do
      // not read as this filter's answer.
      if (!refused) refused = { pager: pagerBox.hidden };
      wrap.hidden = true;
      pagerBox.hidden = true;
      return NEVER;
    }

    /* unrefuse shows the rows and pager again after a refused search,
       when the next read is good. */
    function unrefuse() {
      if (!refused) return;
      pagerBox.hidden = refused.pager;
      wrap.hidden = !last || !shown || !shown.length;
      refused = null;
    }

    function showFailure(res) {
      if (res.status === 400) return badRequest(res);
      failing = true;
      var k = KW.kindOf(res.status);
      alertBox.textContent = "";
      if (res.status === 0 && stopped) return 0;
      // With rows shown, they stay: the banner at the top of the page
      // (refresh.js) says they may be old.
      if (last) {
        tableEl.classList.add("stale");
        // The reader just changed the view: these rows are the old one.
        if (tried !== etagQuery) {
          alertBox.appendChild(KW.listNotes.stale());
        }
      } else {
        wrap.hidden = true;
        empty.textContent = "";
        alertBox.appendChild(KW.panel(k, res.status, function () { poll.now(); }));
      }
      KW.footer(footer, lastOk, true);
      attempt += 1;
      return KW.retryDelay(k, attempt);
    }

    /* ---- the pager ---- */
    var pager = KW.pager($("[data-pager]"), {
      onPage: function (to) { setView(LQ.copy(f, { page: to })); },
      onSize: function (size) { setView(LQ.copy(f, { pageSize: size, page: 1 })); }
    });

    /* "Skip to issues" goes to the first row (or the table). */
    $("[data-skip]").addEventListener("click", function (e) {
      e.preventDefault();
      var first = KW.$("[data-rowlink]", tableEl);
      (first || tableEl).focus();
    });

    writeURL(true);
    filters.show(f);
    quick.paint(f);
    if (openId) drawer.open(openId);
    var poll = KW.poller(function () { return load(false); }, EVERY);
    var tick = window.setInterval(function () { KW.footer(footer, lastOk, failing); }, 5000);
    return {
      /* pop takes Back or Forward between this table with and without
         an issue open beside it; anything else draws the page again. */
      pop: function () {
        if (location.pathname !== "/issues" || listURL() !== "/issues" + LQ.search(f)) return false;
        var id = new URLSearchParams(location.search).get("open") || "";
        KW.rendered = location.pathname + location.search;
        pushedOpen = false;
        if (!LQ.ID.test(id)) id = "";
        if (id && id !== drawer.id()) {
          openId = lastRow = id;
          pushedOpen = true;
          drawer.open(id);
        } else if (!id && drawer.id()) {
          drawer.close(false);
        }
        return true;
      },
      stop: function () {
        KW.place.save(KW.lastList, KW.scrollTop(), lastRow);
        drawer.stop();
        table.stop();
        keep.stop();
        stopped = true;
        if (ctl) ctl.abort();
        poll.stop();
        live.stop();
        filters.stop();
        opts.stop();
        main.classList.remove("wide");
        window.clearInterval(tick);
        window.clearTimeout(slow);
      }
    };
  };
})();
