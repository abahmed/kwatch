/* kwatch dashboard: Active issues and Resolved. Filters live in the URL
   and are applied by the API, so they work past the list limit. */
(function () {
  "use strict";
  var KW = window.KW;
  var EVERY = 20000;
  var LIMIT = 100;

  function readFilters() {
    var p = new URLSearchParams(location.search);
    return { q: p.get("q") || "", severity: p.getAll("severity"),
      namespace: p.get("namespace") || "", state: p.get("state") || "",
      since: p.get("since") || "", sort: p.get("sort") || "" };
  }

  function used(f) {
    return !!(f.q || f.severity.length || f.namespace || f.state || f.since);
  }

  function toQuery(f) {
    var p = new URLSearchParams();
    if (f.q) p.set("q", f.q);
    f.severity.forEach(function (s) { p.append("severity", s); });
    if (f.namespace) p.set("namespace", f.namespace);
    if (f.state) p.set("state", f.state);
    if (f.since) p.set("since", f.since);
    if (f.sort) p.set("sort", f.sort);
    return p;
  }

  /* The API's order is the default: on Active, Urgent first, then
     newest; on Resolved, latest resolved first. Other orders are applied
     here, over the issues already loaded: "severity" ranks Urgent, Needs
     attention, then Low; "recent" puts the most recently seen first. */
  var RANK = { page: 0, notify: 1, digest: 2, silent: 3 };
  function sorted(items, how, kind) {
    var at = function (i) {
      return Date.parse(kind === "resolved" && i.resolvedAt ? i.resolvedAt : i.lastSeen) || 0;
    };
    var rank = function (i) { return RANK[i.severity] === undefined ? 9 : RANK[i.severity]; };
    var out = items.slice();
    if (how === "severity") {
      out.sort(function (a, b) { return (rank(a) - rank(b)) || (at(b) - at(a)); });
    } else if (how === "recent") {
      out.sort(function (a, b) { return at(b) - at(a); });
    }
    return out;
  }

  /* States the Active list can filter on; anything else in the URL is
     ignored rather than sent to the API. */
  var STATES = ["open", "settling", "flapping", "recovering"];

  /* Issues with the same title, severity, state and cause are one
     problem showing up in several places: one row, with the other places
     linked under it. Anything that differs keeps its own row. */
  function grouped(items) {
    var groups = [], byKey = {};
    items.forEach(function (item) {
      var key = [item.severity, item.state, item.title, item.rootCause || ""].join("|");
      if (byKey[key]) byKey[key].rest.push(item);
      else groups.push(byKey[key] = { main: item, rest: [] });
    });
    return groups;
  }

  function sum(counts) {
    var n = 0;
    Object.keys(counts || {}).forEach(function (k) { n += counts[k]; });
    return n;
  }

  function signature(item) {
    return [item.state, item.severity, item.repeatCount, item.title,
      item.lastSeen, item.resolvedAt || ""].join("|");
  }

  /* ---- rows ---- */
  function row(kind, item, isNew) {
    var li = KW.clone("tpl-row").firstElementChild;
    var a = KW.$("a", li);
    a.href = "/issues/" + encodeURIComponent(item.id);
    var chips = KW.$("[data-chips]", li);
    var sev = KW.SEVERITY[item.severity];
    if (kind === "resolved") {
      a.classList.add("sev-resolved");
      chips.appendChild(KW.chip("c-resolved", "Resolved"));
      chips.appendChild(KW.severityChip(item.severity, "was ", " was-chip"));
    } else {
      a.classList.add(sev ? sev.stripe : "sev-low");
      chips.appendChild(KW.severityChip(item.severity));
      // Everything here is happening now; only a different state is news.
      if (item.state !== "open") {
        chips.appendChild(document.createTextNode(" "));
        chips.appendChild(KW.stateChip(item.state));
      }
    }
    if (isNew) {
      a.classList.add("is-new");
      chips.appendChild(KW.el("span", "new-tag", "New"));
    }
    var title = KW.$(".i-title", li);
    title.textContent = item.title;
    if (item.repeatCount > 0) {
      var rep = KW.el("span", "i-rep", "× " + (item.repeatCount + 1));
      rep.title = KW.failedAgain(item.repeatCount);
      title.appendChild(rep);
    }
    // Why, in one line, so most issues need no click to triage.
    if (item.rootCause && item.rootCause !== item.title) {
      var sub = KW.$(".i-sub", li);
      sub.hidden = false;
      sub.textContent = item.rootCause;
    }
    KW.where(item, KW.$(".i-where", li), true);
    var when = KW.$(".i-when", li);
    var small = KW.$("small", when);
    var t = KW.$("time", when);
    if (kind === "resolved" && KW.valid(item.resolvedAt)) {
      when.classList.add("ok");
      when.replaceChild(KW.timeEl(item.resolvedAt, "Resolved "), t);
      small.textContent = "lasted " + KW.lasted(item.firstSeen, item.resolvedAt);
    } else {
      when.replaceChild(KW.timeEl(item.lastSeen, "seen "), t);
      // How long it has been going on reads faster than a start time.
      small.textContent = "for " + KW.lasted(item.firstSeen, new Date(KW.now()).toISOString());
      small.title = "Started " + KW.full(item.firstSeen);
    }
    return li;
  }

  function skeleton(list) {
    list.textContent = "";
    for (var i = 0; i < 3; i++) {
      var li = KW.el("li", "skel");
      li.appendChild(KW.el("span"));
      li.appendChild(KW.el("span"));
      list.appendChild(li);
    }
    list.appendChild(KW.el("li", "sr-only", "Loading"));
    list.hidden = false;
  }

  /* ---- empty states ---- */
  function clearBlock(lines, compact) {
    var box = KW.el("div", "clear" + (compact ? " compact" : ""));
    lines.forEach(function (l) { if (l) box.appendChild(l); });
    return box;
  }

  function scope(status) {
    var w = status.watchedNamespaces || ["*"];
    var ex = status.excludedNamespaces || [];
    function names(list) {
      if (list.length <= 3) return list.join(", ");
      return list.slice(0, 3).join(", ") + " and " + (list.length - 3) + " more";
    }
    if (w.length === 1 && w[0] === "*") {
      return "Watching every namespace" + (ex.length ? " except " + names(ex) : "") + ".";
    }
    return "Watching " + names(w) + ".";
  }

  function activeEmpty(status) {
    if (!status) {
      return clearBlock([KW.el("p", "l1", "Nothing is listed as failing.")], true);
    }
    if (!KW.valid(status.lastScan)) {
      return clearBlock([KW.el("p", "l1", "kwatch hasn't finished its first check yet."),
        KW.el("p", "l2", "Issues will show up here as soon as it has.")], true);
    }
    if (status.lastScanLate) {
      var n = KW.el("div", "notice n-degraded");
      n.setAttribute("role", "status");
      n.appendChild(KW.el("b", "", "kwatch hasn't checked in a while."));
      var p = KW.el("p", "", "Last check: " + KW.rel(status.lastScan) + ". ");
      var a = KW.el("a", "ulink", "See kwatch status.");
      a.href = "/status";
      a.dataset.link = "";
      p.appendChild(a);
      n.appendChild(p);
      return n;
    }
    var l1 = KW.el("p", "l1", "kwatch checked your cluster ");
    l1.appendChild(KW.timeEl(status.lastScan, "", true));
    l1.appendChild(document.createTextNode(" and nothing is failing."));
    var l3 = KW.el("p", "l3");
    var link = KW.el("a", "ulink", "See what was resolved this week");
    link.href = "/resolved";
    link.dataset.link = "";
    l3.appendChild(link);
    return clearBlock([KW.el("h2", "", "All clear."), l1,
      KW.el("p", "l2", scope(status)), l3]);
  }

  function resolvedEmpty() {
    return clearBlock([KW.el("p", "l1", "Nothing resolved this week."),
      KW.el("p", "l2", "When kwatch sees an issue go away, it is listed here for 7 days.")], true);
  }

  /* ---- the view ---- */
  KW.listView = function (main, r) {
    var kind = r.name;
    main.appendChild(KW.clone("tpl-list"));
    var f = readFilters();
    if (kind !== "active" || STATES.indexOf(f.state) < 0) f.state = "";
    var box = KW.$("[data-filterbox]", main);
    var list = KW.$("[data-list]", main);
    var alertBox = KW.$("[data-alert]", main);
    var empty = KW.$("[data-empty]", main);
    var trunc = KW.$("[data-trunc]", main);
    var footer = KW.$("[data-updated]", main);
    var sub = KW.$("[data-sub]", main);
    var prev = null, lastOk = 0, failing = false, attempt = 0, last = null, lastStatus = null;
    // seq ignores a slow answer for older filters; stopped ignores any
    // answer after the page was left.
    var seq = 0, stopped = false, nsKey = null;
    var restore = KW.takeScroll() ||
      (KW.cameFrom === "detail" ? KW.scrollMemo[location.pathname + location.search] : 0);
    var debounce = 0, slow = 0;

    KW.$("[data-h1]", main).textContent = kind === "active" ? "Active issues" : "Resolved";
    if (kind === "resolved") {
      KW.title("Resolved");
      sub.textContent = "kwatch keeps resolved issues for 7 days.";
      KW.$("[data-sevlabel]", main).textContent = "Was";
    }
    KW.$$("[data-only]", main).forEach(function (n) {
      if (n.dataset.only !== kind) n.parentNode.removeChild(n);
    });

    /* filters */
    var q = KW.$('[data-f="q"]', box), ns = KW.$('[data-f="namespace"]', box);
    var st = KW.$('[data-f="state"]', box), since = KW.$('[data-f="since"]', box);
    var sortSel = KW.$('[data-f="sort"]', box);
    if (kind === "resolved") {
      KW.$("[data-sort-default]", sortSel).textContent = "Latest resolved";
      sortSel.removeChild(KW.$('option[value="recent"]', sortSel));
    }
    if (!KW.$('option[value="' + f.sort + '"]', sortSel)) f.sort = "";
    sortSel.value = f.sort;
    q.value = f.q;
    if (st) st.value = f.state;
    if (since) since.value = f.since;
    KW.$$(".fchip", box).forEach(function (b) {
      b.setAttribute("aria-pressed", f.severity.indexOf(b.dataset.sev) >= 0 ? "true" : "false");
    });

    // A select that narrows the list looks set, so it's easy to spot.
    function markSet() {
      [ns, st, since].forEach(function (s) { if (s) s.classList.toggle("is-set", !!s.value); });
    }

    function apply() {
      markSet();
      f = { q: q.value.trim(), namespace: ns.value, state: st ? st.value : "",
        since: since ? since.value : "", sort: sortSel.value,
        severity: KW.$$(".fchip", box).filter(function (b) {
          return b.getAttribute("aria-pressed") === "true";
        }).map(function (b) { return b.dataset.sev; }) };
      var qs = toQuery(f).toString();
      history.replaceState(null, "", location.pathname + (qs ? "?" + qs : ""));
      KW.lastList[kind] = location.pathname + location.search;
      prev = null;
      poll.now();
    }
    q.addEventListener("input", function () {
      window.clearTimeout(debounce);
      debounce = window.setTimeout(apply, 300);
    });
    [ns, st, since].forEach(function (s) { if (s) s.addEventListener("change", apply); });
    // Sorting needs no new data: re-draw what is loaded.
    sortSel.addEventListener("change", function () {
      f.sort = sortSel.value;
      var qs = toQuery(f).toString();
      history.replaceState(null, "", location.pathname + (qs ? "?" + qs : ""));
      KW.lastList[kind] = location.pathname + location.search;
      if (last) render(last, lastStatus);
    });
    KW.$$(".fchip", box).forEach(function (b) {
      b.addEventListener("click", function () {
        b.setAttribute("aria-pressed", b.getAttribute("aria-pressed") === "true" ? "false" : "true");
        apply();
      });
    });
    KW.$("[data-fclear]", box).addEventListener("click", function () {
      q.value = "";
      ns.value = "";
      if (st) st.value = "";
      if (since) since.value = "";
      KW.$$(".fchip", box).forEach(function (b) { b.setAttribute("aria-pressed", "false"); });
      apply();
    });

    function fillNamespaces(names) {
      var want = f.namespace;
      if (want && names.indexOf(want) < 0) names = names.concat([want]).sort();
      // Rebuilt only when the list changed: rebuilding closes an open
      // dropdown and loses the choice being made.
      var key = names.join("\n");
      if (key === nsKey && ns.value === want) return;
      nsKey = key;
      ns.textContent = "";
      ns.appendChild(KW.el("option", "", "All namespaces")).value = "";
      names.forEach(function (n) { ns.appendChild(KW.el("option", "", n)).value = n; });
      ns.value = want;
      markSet();
    }

    function showFailure(res) {
      failing = true;
      var k = KW.kindOf(res.status);
      alertBox.textContent = "";
      if (last && k === "network") {
        var n = KW.el("div", "notice n-degraded");
        n.setAttribute("role", "status");
        n.appendChild(KW.el("b", "", "Can't reach kwatch"));
        // Try now sits in the freshness line at the top, beside this.
        n.appendChild(KW.el("p", "", "The connection dropped. We'll keep trying. Showing what we had " +
          KW.rel(new Date(lastOk + (KW.now() - Date.now())).toISOString()) + "."));
        alertBox.appendChild(n);
        list.classList.add("stale");
      } else {
        list.hidden = true;
        empty.textContent = "";
        trunc.hidden = true;
        alertBox.appendChild(KW.panel(k, res.status, function () { poll.now(); }));
      }
      KW.footer(footer, lastOk, true, function () { poll.now(); });
      attempt += 1;
      return KW.retryDelay(k, attempt);
    }

    function render(data, status) {
      failing = false;
      attempt = 0;
      alertBox.textContent = "";
      list.classList.remove("stale");
      var unfiltered = sum(data.counts);
      box.hidden = unfiltered < 5 && !used(f);
      // How many of each severity, before filters: the spread at a glance.
      KW.$$(".fchip", box).forEach(function (b) {
        var n = (data.counts || {})[b.dataset.sev] || 0;
        var tag = KW.$("[data-n]", b);
        tag.textContent = n;
        if (!b.dataset.name) b.dataset.name = b.textContent.replace(/\d+$/, "").trim();
        b.setAttribute("aria-label", b.dataset.name + ", " + KW.plural(n, "issue", "issues"));
        b.classList.toggle("is-zero", n === 0);
      });
      var fa = KW.$("[data-factive]", box);
      fa.hidden = !used(f);
      KW.$("[data-fsummary]", box).textContent = "Showing " + data.total + " of " + unfiltered;
      var countText = data.truncated ? "Showing " + data.incidents.length + " of " + data.total :
        KW.plural(data.total, "issue", "issues") + (f.namespace ? " in " + f.namespace : "");
      if (kind === "active") {
        KW.updateBadge(data.counts);
        var active = KW.activeTotal || 0;
        document.title = active ? "(" + active + ") Active issues · kwatch" : "All clear · kwatch";
        sub.textContent = countText;
        sub.hidden = data.total === 0 && !used(f);
      } else {
        sub.textContent = data.total === 0 && !used(f) ? "kwatch keeps resolved issues for 7 days." :
          countText + " · kwatch keeps them for 7 days";
      }
      var seen = {};
      // A refresh rebuilds the rows: keep keyboard focus on the same issue.
      var focused = document.activeElement;
      var focusHref = focused && list.contains(focused) ? focused.getAttribute("href") : "";
      list.textContent = "";
      var urgentNew = [];
      var ids = [];
      function isNewItem(item) {
        var sig = signature(item);
        seen[item.id] = sig;
        var isNew = prev !== null && prev[item.id] !== sig;
        if (isNew && prev[item.id] === undefined && item.severity === "page" && kind === "active") {
          urgentNew.push(item.title);
        }
        return isNew;
      }
      grouped(sorted(data.incidents, f.sort, kind)).forEach(function (g) {
        var li = row(kind, g.main, isNewItem(g.main));
        ids.push(g.main.id);
        if (g.rest.length) {
          var also = KW.el("p", "also", "Also in " + g.rest.length + (g.rest.length === 1 ? " more place: " : " more places: "));
          g.rest.forEach(function (item, i) {
            var fresh = isNewItem(item);
            ids.push(item.id);
            if (i) also.appendChild(document.createTextNode(", "));
            var link = KW.el("a", "ulink mono");
            KW.where(item, link);
            link.href = "/issues/" + encodeURIComponent(item.id);
            link.dataset.link = "";
            also.appendChild(link);
            if (fresh) also.appendChild(KW.el("span", "new-tag", "New"));
          });
          li.appendChild(also);
        }
        list.appendChild(li);
      });
      // The order on screen, for Previous / Next on an issue.
      KW.listIds[kind] = ids;
      if (focusHref) {
        var again = KW.$$("a", list).filter(function (a) {
          return a.getAttribute("href") === focusHref;
        })[0];
        if (again) again.focus({ preventScroll: true });
      }
      prev = seen;
      if (urgentNew.length) KW.announce("New urgent issue: " + urgentNew.join("; "));
      empty.textContent = "";
      if (data.incidents.length === 0 && used(f)) {
        var none = KW.el("li", "nomatch", "No issues match these filters. ");
        var clr = KW.el("button", "link", "Clear filters");
        clr.type = "button";
        clr.addEventListener("click", function () { KW.$("[data-fclear]", box).click(); });
        none.appendChild(clr);
        list.appendChild(none);
      } else if (data.incidents.length === 0) {
        empty.appendChild(kind === "active" ? activeEmpty(status) : resolvedEmpty());
      }
      list.hidden = list.children.length === 0;
      trunc.hidden = !data.truncated;
      KW.$("p", trunc).textContent = "Showing the newest " + data.incidents.length + " of " +
        data.total + ". Pick a namespace to narrow it down.";
      KW.footer(footer, lastOk, false);
      if (restore) {
        window.scrollTo(0, restore);
        restore = 0;
      }
    }

    function load() {
      var query = toQuery(f);
      query.delete("sort");
      query.delete("state");
      query.append("state", kind);
      if (f.state && kind === "active") query.append("state", f.state);
      query.set("limit", String(LIMIT));
      window.clearTimeout(slow);
      if (!last) slow = window.setTimeout(function () { skeleton(list); }, 300);
      var mine = ++seq;
      return Promise.all([
        KW.api("/api/incidents?" + query.toString()),
        kind === "active" ? KW.api("/api/status") : KW.refreshBadge(),
        KW.api("/api/namespaces?state=" + kind)
      ]).then(function (out) {
        if (stopped || mine !== seq) return 0;
        window.clearTimeout(slow);
        var res = out[0];
        if (KW.authFailed(res)) return 0;
        if (res.status !== 200 || !res.data) return showFailure(res);
        lastOk = Date.now();
        last = res.data;
        var status = out[1] && out[1].status === 200 ? out[1].data : null;
        lastStatus = status;
        if (status) KW.self = { namespace: status.kwatchNamespace || "kwatch",
          deployment: status.kwatchDeployment || "kwatch" };
        if (out[2].status === 200 && out[2].data) fillNamespaces(out[2].data.namespaces || []);
        render(res.data, status);
        return 0;
      });
    }

    var poll = KW.poller(load, EVERY);
    var tick = window.setInterval(function () {
      if (!failing) KW.footer(footer, lastOk, false);
    }, 5000);
    return {
      stop: function () {
        stopped = true;
        poll.stop();
        window.clearInterval(tick);
        window.clearTimeout(debounce);
        window.clearTimeout(slow);
      }
    };
  };
})();
