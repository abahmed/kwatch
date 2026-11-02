/* kwatch dashboard: "What kwatch is watching" on the Overview, a
   folded section drawn from /api/coverage: the namespaces watched and
   left out, the kinds of objects watched and fully read, the kinds it
   can't read and why, and when it finished reading the cluster. It
   reads the API with the Overview (an unchanged answer costs a 304),
   since the health line says when kwatch isn't allowed to read some
   kinds; then the section opens by itself, once. */
(function () {
  "use strict";
  var KW = window.KW;
  var SHOWN = 20;
  var REASONS = {
    permission_denied: "kwatch isn't allowed to read them (its role lacks permission)",
    cache_sync_pending: "still being read for the first time",
    cache_sync_timeout: "took too long to read",
    cache_sync_failed: "couldn't be read",
    disabled_by_config: "turned off in kwatch's settings",
    api_unavailable: "not served by this cluster",
    watch_budget_exceeded: "over the number of kinds kwatch watches",
    object_cap_reached: "too many objects to watch"
  };

  var num = function (n) { return KW.num(n); };

  /* names: up to SHOWN pills and "+N more". */
  function names(list, total) {
    var box = KW.el("div", "pills");
    (list || []).slice(0, SHOWN).forEach(function (n) {
      box.appendChild(KW.el("span", "pill mono", n));
    });
    var rest = (total || 0) - Math.min(SHOWN, (list || []).length);
    if (rest > 0) box.appendChild(KW.el("span", "pill cov-more", "+" + num(rest) + " more"));
    return box;
  }

  function section(title) {
    var s = KW.el("div", "cov-part");
    s.appendChild(KW.el("h3", "cov-h", title));
    return s;
  }

  function namespaces(ns) {
    var s = section("Namespaces");
    if (ns.all && !ns.excluded) {
      s.appendChild(KW.el("p", "", "Every namespace: " + num(ns.watched) + " now."));
      return s;
    }
    s.appendChild(KW.el("p", "", num(ns.watched) + " watched" +
      (ns.excluded ? ", " + num(ns.excluded) + " left out" : "") + "."));
    if (!ns.all && ns.watched) s.appendChild(names(ns.watchedNames, ns.watched));
    if (ns.excluded) {
      s.appendChild(KW.el("p", "cov-sub", "Left out"));
      s.appendChild(names(ns.excludedNames, ns.excluded));
    }
    return s;
  }

  function gaps(title, list) {
    var s = section(title);
    var ul = KW.el("ul", "cov-gaps");
    list.slice(0, SHOWN).forEach(function (g) {
      var li = KW.el("li");
      li.appendChild(KW.el("span", "mono", g.kind));
      li.appendChild(document.createTextNode(": " + (REASONS[g.reason] || g.detail || g.reason) +
        (g.required ? ". kwatch needs this kind." : ".")));
      ul.appendChild(li);
    });
    s.appendChild(ul);
    if (list.length > SHOWN) s.appendChild(KW.el("p", "cov-sub", "+" + num(list.length - SHOWN) + " more"));
    return s;
  }

  function kinds(k) {
    var s = section("Kinds of objects");
    s.appendChild(KW.el("p", "", num(k.watched) + " watched, " + num(k.synced) +
      " fully read."));
    return s;
  }

  function draw(body, c) {
    body.textContent = "";
    var k = c.kinds || {};
    var line = c.syncedAt ? "Finished reading the cluster " : "Still reading the cluster for the first time.";
    var head = KW.el("p", "cov-lead", line);
    if (c.syncedAt) head.appendChild(KW.timeEl(c.syncedAt));
    if (c.discoveryComplete === false) {
      head.appendChild(document.createTextNode(" The last look for new kinds was incomplete."));
    }
    body.appendChild(head);
    var grid = KW.el("div", "cov-grid");
    grid.appendChild(namespaces(c.namespaces || {}));
    grid.appendChild(kinds(k));
    var unread = (k.unreadable || []);
    if (unread.length) grid.appendChild(gaps("Can't read", unread));
    if ((k.unsynced || []).length) grid.appendChild(gaps("Not read yet", k.unsynced));
    if ((k.notWatched || []).length) grid.appendChild(gaps("Not watched", k.notWatched));
    body.appendChild(grid);
  }

  function denied(c) {
    return ((c.kinds && c.kinds.unreadable) || []).some(function (g) {
      return g.reason === "permission_denied";
    });
  }

  /* coverageView wires the folded section; load reads it again and
     data() is the last answer (null before one). */
  KW.coverageView = function (main) {
    var fold = KW.$("[data-cov]", main);
    var body = KW.$("[data-cov-body]", main);
    var etag = "", stopped = false, last = null, opened = false;
    function load() {
      return KW.api("/api/coverage", { etag: etag }).then(function (res) {
        if (stopped) return;
        if (res.status === 200 && res.data) {
          etag = res.etag;
          last = res.data;
          draw(body, res.data);
          if (!opened && denied(res.data)) fold.open = true;
          opened = true;
        } else if (res.status !== 304 && !last) {
          body.textContent = "kwatch can't say what it watches right now.";
        }
      });
    }
    function onClick(e) {
      var a = e.target.closest && e.target.closest("[data-cov-open]");
      if (!a) return;
      e.preventDefault();
      fold.open = true;
      fold.scrollIntoView({ block: "start" });
      KW.$("summary", fold).focus();
    }
    main.addEventListener("click", onClick);
    return { load: load, data: function () { return last; },
      stop: function () {
        stopped = true;
        main.removeEventListener("click", onClick);
      } };
  };
})();
