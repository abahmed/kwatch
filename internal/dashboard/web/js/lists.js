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
      since: p.get("since") || "" };
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
    return p;
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
      chips.appendChild(document.createTextNode(" "));
      chips.appendChild(KW.stateChip(item.state));
    }
    if (isNew) {
      a.classList.add("is-new");
      chips.appendChild(KW.el("span", "new-tag", "New"));
    }
    KW.$(".i-title", li).textContent = item.title;
    if (item.repeatCount > 0) {
      var sub = KW.$(".i-sub", li);
      sub.hidden = false;
      sub.textContent = KW.failedAgain(item.repeatCount);
    }
    KW.where(item, KW.$(".i-where", li));
    var when = KW.$(".i-when", li);
    var small = KW.$("small", when);
    var t = KW.$("time", when);
    if (kind === "resolved" && KW.valid(item.resolvedAt)) {
      when.classList.add("ok");
      when.replaceChild(KW.timeEl(item.resolvedAt, "Resolved "), t);
      small.textContent = "lasted " + KW.lasted(item.firstSeen, item.resolvedAt);
    } else {
      when.replaceChild(KW.timeEl(item.lastSeen), t);
      small.textContent = "Started " + KW.hhmm(new Date(item.firstSeen));
      small.title = KW.full(item.firstSeen);
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
    var box = KW.$("[data-filterbox]", main);
    var list = KW.$("[data-list]", main);
    var alertBox = KW.$("[data-alert]", main);
    var empty = KW.$("[data-empty]", main);
    var trunc = KW.$("[data-trunc]", main);
    var footer = KW.$("[data-updated]", main);
    var sub = KW.$("[data-sub]", main);
    var countLine = KW.el("p", "sub muted count-line");
    countLine.hidden = true;
    var prev = null, lastOk = 0, failing = false, attempt = 0, last = null;
    var debounce = 0, slow = 0;

    KW.$("[data-h1]", main).textContent = kind === "active" ? "Active issues" : "Resolved";
    if (kind === "resolved") {
      KW.title("Resolved");
      sub.textContent = "Issues from the last 7 days. kwatch keeps them for a week.";
      KW.$("[data-sevlabel]", main).textContent = "Was";
      box.parentNode.insertBefore(countLine, list);
    }
    KW.$$("[data-only]", main).forEach(function (n) {
      if (n.dataset.only !== kind) n.parentNode.removeChild(n);
    });

    /* filters */
    var q = KW.$('[data-f="q"]', box), ns = KW.$('[data-f="namespace"]', box);
    var st = KW.$('[data-f="state"]', box), since = KW.$('[data-f="since"]', box);
    var more = KW.$("[data-more]", box), moreBox = KW.$("[data-more-box]", box);
    q.value = f.q;
    if (st) st.value = f.state;
    if (since) since.value = f.since;
    KW.$$(".fchip", box).forEach(function (b) {
      b.setAttribute("aria-pressed", f.severity.indexOf(b.dataset.sev) >= 0 ? "true" : "false");
    });
    function setMore(open) {
      moreBox.hidden = !open;
      more.setAttribute("aria-expanded", open ? "true" : "false");
      more.textContent = open ? "Fewer filters" : "More filters";
    }
    setMore(!!(f.namespace || f.state || f.since));
    more.addEventListener("click", function () { setMore(moreBox.hidden); });

    function apply() {
      f = { q: q.value.trim(), namespace: ns.value, state: st ? st.value : "",
        since: since ? since.value : "",
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
      ns.textContent = "";
      ns.appendChild(KW.el("option", "", "All namespaces")).value = "";
      names.forEach(function (n) { ns.appendChild(KW.el("option", "", n)).value = n; });
      ns.value = want;
    }

    function showFailure(res) {
      failing = true;
      var k = KW.kindOf(res.status);
      alertBox.textContent = "";
      if (last && k === "network") {
        var n = KW.el("div", "notice n-degraded");
        n.setAttribute("role", "status");
        n.appendChild(KW.el("b", "", "Can't reach kwatch"));
        n.appendChild(KW.el("p", "", "The connection dropped. We'll keep trying. Showing what we had " +
          KW.rel(new Date(lastOk + (KW.now() - Date.now())).toISOString()) + "."));
        var p = KW.el("p");
        var b = KW.el("button", "link", "Try now");
        b.type = "button";
        b.addEventListener("click", function () { poll.now(); });
        p.appendChild(b);
        n.appendChild(p);
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
        countLine.textContent = countText;
        countLine.hidden = data.total === 0 && !used(f);
      }
      var seen = {};
      list.textContent = "";
      var urgentNew = [];
      data.incidents.forEach(function (item) {
        var sig = signature(item);
        seen[item.id] = sig;
        var isNew = prev !== null && prev[item.id] !== sig;
        if (isNew && prev[item.id] === undefined && item.severity === "page" && kind === "active") {
          urgentNew.push(item.title);
        }
        list.appendChild(row(kind, item, isNew));
      });
      prev = seen;
      if (urgentNew.length) KW.announce("New urgent issue: " + urgentNew.join("; "));
      empty.textContent = "";
      if (data.incidents.length === 0 && used(f)) {
        list.appendChild(KW.el("li", "nomatch", "No issues match these filters."));
      } else if (data.incidents.length === 0) {
        empty.appendChild(kind === "active" ? activeEmpty(status) : resolvedEmpty());
      }
      list.hidden = list.children.length === 0;
      trunc.hidden = !data.truncated;
      KW.$("p", trunc).textContent = "Showing the newest " + data.incidents.length + " of " +
        data.total + ". Pick a namespace to narrow it down.";
      KW.footer(footer, lastOk, false);
    }

    function load() {
      var query = toQuery(f);
      query.delete("state");
      query.append("state", kind);
      if (f.state && kind === "active") query.append("state", f.state);
      query.set("limit", String(LIMIT));
      window.clearTimeout(slow);
      if (!last) slow = window.setTimeout(function () { skeleton(list); }, 300);
      return Promise.all([
        KW.api("/api/incidents?" + query.toString()),
        kind === "active" ? KW.api("/api/status") : KW.refreshBadge(),
        KW.api("/api/namespaces?state=" + kind)
      ]).then(function (out) {
        window.clearTimeout(slow);
        var res = out[0];
        if (KW.authFailed(res)) return 0;
        if (res.status !== 200 || !res.data) return showFailure(res);
        lastOk = Date.now();
        last = res.data;
        var status = out[1] && out[1].status === 200 ? out[1].data : null;
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
        poll.stop();
        window.clearInterval(tick);
        window.clearTimeout(debounce);
        window.clearTimeout(slow);
      }
    };
  };
})();
