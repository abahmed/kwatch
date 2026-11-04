/* kwatch dashboard: the Issues table. One row per issue, the columns
   the reader chose, headers that sort (with a menu and a resizable
   edge: colmenu.js), and rows that update their cells in place so a
   refresh never moves what is being read. A click on a row opens its
   issue beside it (drawer.js). Under 900px wide the rows become
   stacked cards (issues.css). issues.js drives it. */
(function () {
  "use strict";
  var KW = window.KW;
  /* The longest title text kept before a search match, so the match
     shows before the cell cuts the title with "…". */
  var LEAD = 24;

  function href(id) { return "/issues/" + encodeURIComponent(id); }

  function label(key) {
    var c = KW.prefs.column(key);
    return c ? c.label : key;
  }

  /* The "When" column: how long an active issue has been going on,
     when a resolved one resolved. */
  var WHEN = { active: "Going on", resolved: "Resolved",
    all: "Going on or resolved" };
  function whenSort(status) { return status === "active" ? "duration" : "last"; }

  function objectText(item) {
    return KW.obj(item.workloadKind, item.namespace, item.workload);
  }

  /* breaks lets a long object wrap after "/" and "." ("deployment/
     api.v2"), never inside a word. */
  function breaks(node) {
    var walk = document.createTreeWalker(node, NodeFilter.SHOW_TEXT), list = [];
    while (walk.nextNode()) list.push(walk.currentNode);
    list.forEach(function (t) {
      var parts = t.nodeValue.split(/(?<=[\/.])/);
      if (parts.length < 2) return;
      parts.forEach(function (p, i) {
        t.parentNode.insertBefore(document.createTextNode(p), t);
        if (i < parts.length - 1) t.parentNode.insertBefore(document.createElement("wbr"), t);
      });
      t.parentNode.removeChild(t);
    });
  }

  /* dash is an empty value; cards leave such a line out. */
  function dash(td) {
    td.classList.add("none");
    return KW.el("span", "muted", "–");
  }

  function found(text, terms) {
    var low = (text || "").toLowerCase();
    return terms.some(function (t) { return low.indexOf(t) >= 0; });
  }

  /* titleText: the title, starting a little before the first search
     match when that match would be cut off at the end of the cell. */
  function titleText(title, terms) {
    var low = title.toLowerCase(), at = -1;
    terms.forEach(function (t) {
      var i = low.indexOf(t);
      if (i >= 0 && (at < 0 || i < at)) at = i;
    });
    return at > LEAD * 2 ? "…" + title.slice(at - LEAD) : title;
  }

  /* marks: after the title, how often it failed again (× 3), New
     or Changed since the last visit, "Settled before alerting", "Stopped tracking"; "in the cause" when the search
     matched only the likely cause. */
  function marks(td, item, ctx) {
    if (item.repeatCount > 0) {
      var rep = td.appendChild(KW.el("span", "i-rep", "× " + KW.num(item.repeatCount + 1)));
      rep.title = KW.failedAgain(item.repeatCount);
    }
    var m = ctx.marks[item.id];
    if (m) td.appendChild(KW.visitChip(m));
    if (KW.isUntold(item)) td.appendChild(KW.untoldMark());
    if (item.stoppedTracking) {
      var stop = td.appendChild(KW.el("span", "i-stopped", "Stopped tracking"));
      stop.title = "kwatch stopped tracking this issue while it was still failing.";
    }
    if (ctx.terms.length && !found(item.title + " " + item.namespace + " " +
        objectText(item), ctx.terms) && found(item.rootCause, ctx.terms)) {
      var why = td.appendChild(KW.el("span", "i-why", "in the cause"));
      why.title = "The search matched the likely cause: " + item.rootCause;
    }
  }

  /* CELLS fill one cell of an issue row. */
  var CELLS = {
    sev: function (td, item) { td.appendChild(KW.severityChip(item.severity)); },
    title: function (td, item, ctx) {
      var a = KW.el("a", "it-link");
      KW.highlight(a, titleText(item.title, ctx.terms), ctx.terms);
      a.href = href(item.id);
      a.dataset.link = "";
      a.dataset.rowlink = "";
      a.dataset.fk = "row:" + item.id;
      a.title = item.title + (item.rootCause && item.rootCause !== item.title ?
        "\nLikely cause: " + item.rootCause : "");
      td.appendChild(a);
      marks(td, item, ctx);
    },
    ns: function (td, item, ctx) {
      var span = td.appendChild(KW.el("span", item.namespace ? "mono" : ""));
      if (item.namespace) KW.highlight(span, item.namespace, ctx.terms);
      else span.textContent = "whole cluster";
      td.title = item.namespace || "whole cluster";
    },
    obj: function (td, item, ctx) {
      if (!item.workload) return td.appendChild(dash(td));
      var span = td.appendChild(KW.el("span", "mono"));
      KW.highlight(span, objectText(item), ctx.terms);
      breaks(span);
      td.title = objectText(item);
      // A card leaves the line out when the title already names it.
      td.classList.toggle("in-title", String(item.title).indexOf(item.workload) >= 0);
    },
    status: function (td, item) { td.appendChild(KW.stateChip(item.state)); },
    started: function (td, item) {
      if (KW.valid(item.firstSeen)) td.appendChild(KW.timeEl(item.firstSeen));
    },
    last: function (td, item) {
      if (item.state === "resolved" && KW.valid(item.resolvedAt)) {
        td.appendChild(KW.timeEl(item.resolvedAt));
        td.classList.add("ok");
        td.dataset.label = "Resolved";
        return;
      }
      if (!KW.valid(item.firstSeen)) return td.appendChild(dash(td));
      td.textContent = KW.forLong(item.firstSeen);
      td.dataset.since = item.firstSeen;
      td.dataset.tick = "last";
      td.dataset.label = "Going on";
      td.title = "Started " + KW.full(item.firstSeen);
    },
    dur: function (td, item) {
      if (!KW.valid(item.firstSeen)) return td.appendChild(dash(td));
      var done = item.state === "resolved" && KW.valid(item.resolvedAt);
      var end = done ? item.resolvedAt : new Date(KW.now()).toISOString();
      if (!done) {
        td.dataset.since = item.firstSeen;
        td.dataset.tick = "dur";
      }
      td.textContent = KW.lasted(item.firstSeen, end);
      td.title = "Started " + KW.full(item.firstSeen);
    },
    ack: function (td, item) {
      if (!item.ackedBy) return td.appendChild(dash(td));
      td.textContent = "Yes";
      td.title = "Acknowledged on " + item.ackedBy;
    }
  };

  function markSig(item, ctx) {
    return (item.rev || "") + "|" + JSON.stringify(ctx.marks[item.id] || "");
  }

  function fillRow(tr, item, ctx) {
    tr.textContent = "";
    var sev = KW.SEVERITY[item.severity];
    tr.className = "it-row " + (ctx.status === "resolved" ? "sev-none" :
      item.state === "resolved" ? "sev-resolved" : sev ? sev.stripe : "sev-low");
    tr.dataset.id = item.id;
    // rev plus the mark: a mark can change while the issue does not.
    tr.dataset.rev = markSig(item, ctx);
    ctx.cols.forEach(function (key) {
      var td = KW.el("td", "c-" + key);
      td.dataset.label = label(key);
      CELLS[key](td, item, ctx);
      tr.appendChild(td);
    });
  }

  /* newTab: Ctrl, Cmd, Shift or the middle button open the issue page
     in a new tab, like the title link itself. */
  function newTab(e) {
    return e.ctrlKey || e.metaKey || e.shiftKey || e.button === 1;
  }

  /* SORTS: what ascending and descending mean for each sort field, in
     the words the heading's tooltip and menu use. */
  var SORTS = {
    severity: ["Least urgent first", "Most urgent first"],
    title: ["A→Z", "Z→A"], namespace: ["A→Z", "Z→A"], object: ["A→Z", "Z→A"],
    status: ["Happening now first", "Resolved first"],
    started: ["Oldest first", "Newest first"], last: ["Oldest first", "Newest first"],
    duration: ["Shortest first", "Longest first"],
    repeats: ["Fewest repeats first", "Most repeats first"],
    acked: ["Not acknowledged first", "Acknowledged first"]
  };

  /* ---- the table ---- */
  /* issueTable draws into table. hooks: sort(field, dir) orders by a
     column (dir "" flips it), columns() says the columns changed,
     open(id) opens an issue beside the table (drawer.js). */
  KW.issueTable = function (table, hooks) {
    var head = KW.$("thead", table), body = KW.$("tbody", table);
    var ctx = null;

    function sortButton(th, field, text, f) {
      var b = KW.el("button", "it-sort", text);
      b.type = "button";
      b.dataset.sort = field;
      b.dataset.fk = "sort:" + field;
      var words = SORTS[field] || ["Ascending", "Descending"];
      if (f.sort === field) {
        var desc = f.dir ? f.dir === "desc" : DESCENDS[field];
        th.setAttribute("aria-sort", desc ? "descending" : "ascending");
        b.title = "Sorted: " + words[desc ? 1 : 0] + ". Click for " +
          words[desc ? 0 : 1].toLowerCase() + ".";
      } else {
        b.title = "Sort by " + text.toLowerCase();
      }
      b.addEventListener("click", function () { hooks.sort(field, ""); });
      th.appendChild(b);
    }

    function move(key, by) {
      var order = KW.prefs.order(), at = order.indexOf(key);
      var shown = KW.prefs.columns(), next = shown[shown.indexOf(key) + by];
      if (!next || next === "sev") return;
      order.splice(at, 1);
      order.splice(order.indexOf(next) + (by > 0 ? 1 : 0), 0, key);
      KW.prefs.setColumns(shown, order);
      hooks.columns(key);
    }

    var headHooks = {
      sort: function (field, dir) { hooks.sort(field, dir); },
      hide: function (key) {
        KW.prefs.setColumns(KW.prefs.columns().filter(function (k) { return k !== key; }));
        hooks.columns();
        KW.announce(KW.prefs.column(key).label + " hidden. Columns above the table shows it again.");
      },
      move: move
    };

    function drawHead(cols, f) {
      var tr = KW.el("tr");
      cols.forEach(function (key) {
        var c = Object.assign({}, KW.prefs.column(key));
        var th = KW.el("th", "c-" + key);
        th.scope = "col";
        if (key === "last") c.sort = whenSort(f.status);
        var text = key === "last" ? WHEN[f.status] : c.short || c.label;
        if (c.sort) sortButton(th, c.sort, text, f);
        else th.appendChild(KW.el("span", "it-th", text));
        th.title = c.short ? c.label : text;
        KW.columnHead.head(th, c, table, cols, headHooks);
        tr.appendChild(th);
      });
      head.textContent = "";
      head.appendChild(tr);
    }

    /* "Going on" and "Duration" of open issues grow with the clock, and
       KW.tickTimes only knows <time> elements, so they tick here; the
       timer ends once the table has left the page. */
    var ticker = window.setInterval(function () {
      if (!table.isConnected) return stop();
      KW.$$("td[data-tick]", body).forEach(function (td) {
        td.textContent = td.dataset.tick === "last" ? KW.forLong(td.dataset.since) :
          KW.lasted(td.dataset.since, new Date(KW.now()).toISOString());
      });
    }, 30000);

    /* stop ends the timer: each visit to the page makes a table, and the
       timer would keep all of it alive. */
    function stop() { window.clearInterval(ticker); }

    /* draw replaces every row with page (a list response) and returns
       the issue IDs in the order shown. The Resolved list has no
       Status column: every row there is resolved. */
    function draw(page, f, o) {
      var cols = o.cols.filter(function (k) {
        return !(k === "status" && f.status === "resolved");
      });
      ctx = { cols: cols, marks: o.marks, terms: o.terms, status: f.status };
      KW.columnHead.close(false);
      drawHead(cols, f);
      table.classList.toggle("compact", o.density === "compact");
      body.textContent = "";
      var ids = [];
      (page.incidents || []).forEach(function (item) {
        ids.push(item.id);
        var tr = KW.el("tr");
        fillRow(tr, item, ctx);
        body.appendChild(tr);
      });
      KW.columnHead.size(table, cols);
      return ids;
    }

    /* patch refreshes, in place, the rows of issues in byId whose
       content changed, and dims rows listed in gone. Nothing moves. */
    function patch(byId, gone, marks2) {
      if (!ctx) return;
      ctx.marks = marks2;
      KW.$$("tr[data-id]", body).forEach(function (tr) {
        var item = byId[tr.dataset.id];
        tr.classList.toggle("is-gone", !!gone[tr.dataset.id]);
        tr.title = gone[tr.dataset.id] ? "No longer in this list. Show the new list to update it." : "";
        if (!item || markSig(item, ctx) === tr.dataset.rev) return;
        var had = tr.contains(document.activeElement);
        fillRow(tr, item, ctx);
        if (had) KW.$("[data-rowlink]", tr).focus({ preventScroll: true });
        tr.classList.add("is-updated");
      });
    }

    /* A click anywhere on a row, or on its title (Enter on it too),
       opens the issue beside the table; with Ctrl, Cmd, Shift or the
       middle button its page opens in a new tab. Other links and
       buttons in a row do their own thing, and a click that ends a
       text selection opens nothing. */
    body.addEventListener("click", function (e) {
      var tr = e.target.closest("tr[data-id]"), link = e.target.closest("a, button");
      if (!tr || (link && !link.hasAttribute("data-rowlink")) ||
          KW.selecting(tr)) return;
      if (newTab(e)) {
        if (!link) window.open(href(tr.dataset.id), "_blank", "noopener");
        return;
      }
      e.preventDefault();
      hooks.open(tr.dataset.id);
    });
    body.addEventListener("auxclick", function (e) {
      var tr = e.target.closest("tr[data-id]");
      if (e.button !== 1 || !tr || e.target.closest("a, button")) return;
      e.preventDefault();
      window.open(href(tr.dataset.id), "_blank", "noopener");
    });

    return { draw: draw, patch: patch, stop: stop };
  };

  /* DESCENDS: the direction a column starts in (as the API's). */
  var DESCENDS = { severity: true, started: true, last: true, duration: true,
    repeats: true };
  KW.issueTable.DESCENDS = DESCENDS;
  KW.issueTable.SORTS = SORTS;
})();
