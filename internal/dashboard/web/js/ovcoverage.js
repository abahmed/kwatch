/* kwatch dashboard: what kwatch watches, for the kwatch page
   (kwatchpage.js), drawn from /api/coverage: the namespaces watched and
   left out, the kinds of objects watched and fully read, the kinds it
   can't read and why, when it finished reading the cluster, and the
   ClusterRole rules to copy for the kinds its role may not read. Also
   the plain words for why kwatch last stopped and for byte sizes. */
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
      s.appendChild(KW.el("p", "", "All namespaces watched (" + num(ns.watched) + ")."));
      return s;
    }
    s.appendChild(KW.el("p", "", KW.plural(ns.watched, "namespace", "namespaces") + " watched" +
      (ns.excluded ? ", " + num(ns.excluded) + " left out" : "") + "."));
    if (!ns.all && ns.watched) s.appendChild(names(ns.watchedNames, ns.watched));
    if (ns.excluded) {
      s.appendChild(KW.el("p", "cov-sub", "Left out"));
      s.appendChild(names(ns.excludedNames, ns.excluded));
    }
    return s;
  }

  function gaps(title, list, total) {
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
    // The real count of the kind (kindTotals), not only the list sent.
    var rest = Math.max(total || 0, list.length) - Math.min(SHOWN, list.length);
    if (rest > 0) s.appendChild(KW.el("p", "cov-sub", "+" + num(rest) + " more"));
    return s;
  }

  /* kinds: "233 watched · all read · 2 more can't be read": the kinds
     it can't read are not among those watched. */
  function kinds(k, unread, perm) {
    var s = section("Kinds of objects");
    s.appendChild(KW.el("p", "", num(k.watched) + " watched · " +
      (k.synced >= k.watched ? "all read" : num(k.synced) + " read so far") +
      (unread ? " · " + num(unread) + " more can't be read" + (perm ? " (permission)" : "") : "") + "."));
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
    var tot = c.kindTotals || {};
    var unread = (k.unreadable || []);
    grid.appendChild(kinds(k, Math.max(tot.unreadable || 0, unread.length), unread.length > 0 &&
      unread.every(function (g) { return g.reason === "permission_denied"; })));
    if (unread.length) grid.appendChild(gaps("Can't read", unread, tot.unreadable));
    if ((k.unsynced || []).length) grid.appendChild(gaps("Not read yet", k.unsynced, tot.unsynced));
    if ((k.notWatched || []).length) grid.appendChild(gaps("Not watched", k.notWatched, tot.notWatched));
    body.appendChild(grid);
    var fix = rbac(k);
    if (fix) body.appendChild(fix);
  }

  /* rules: the ClusterRole rules that would let kwatch read what its
     role lacks: the server's missingRules, else the rule of each kind
     it isn't allowed to read. */
  function rules(k) {
    var out = (k.missingRules || []).slice();
    if (!out.length) {
      (k.unreadable || []).forEach(function (g) {
        if (g.reason === "permission_denied" && g.rule) out.push(g.rule);
      });
    }
    return out.filter(function (r) { return r && (r.resources || []).length; });
  }

  function yaml(list) {
    var lines = ["rules:"];
    list.forEach(function (r) {
      lines.push("  - apiGroups: " + JSON.stringify(r.apiGroups || [""]));
      lines.push("    resources: " + JSON.stringify(r.resources));
      lines.push("    verbs: " + JSON.stringify(r.verbs || ["get", "list", "watch"]));
    });
    return lines.join("\n");
  }

  /* rbac: the rules to add, to copy, for kinds kwatch isn't allowed to
     read; null when nothing is missing. */
  function rbac(k) {
    var list = rules(k);
    if (!list.length) return null;
    var box = KW.el("div", "cov-part cov-rbac");
    box.appendChild(KW.el("h3", "cov-h", "Permission to add"));
    box.appendChild(KW.el("p", "", "Add these rules to kwatch's ClusterRole, " +
      "or use rbac.mode=full in the Helm chart."));
    var text = yaml(list);
    var snip = box.appendChild(KW.el("div", "cov-yaml"));
    snip.appendChild(KW.el("pre")).appendChild(KW.el("code", "", text));
    var b = snip.appendChild(KW.el("button", "btn btn-sm", "Copy"));
    b.type = "button";
    b.addEventListener("click", function () { KW.copy(b, text, "Copy"); });
    return box;
  }

  function denied(c) {
    return ((c.kinds && c.kinds.unreadable) || []).some(function (g) {
      return g.reason === "permission_denied";
    });
  }

  /* ---- kwatch itself ---- */
  /* STOPPED: why kwatch's last session ended, in plain words. */
  KW.STOPPED = {
    graceful_shutdown: "normal restart", leader_handoff: "another kwatch took over",
    internal_failure: "internal error", oom_killed: "ran out of memory",
    eviction: "evicted", node_disruption: "its node went away",
    deployment_rollout: "new version rolled out", api_unavailable: "API server unreachable",
    unknown: "unknown"
  };

  /* KW.size: bytes in words: "812 B", "4.2 MB". */
  KW.size = function (n) {
    if (!(n >= 1024)) return Math.max(0, n || 0) + " B";
    var units = ["KB", "MB", "GB"], i = -1;
    do { n /= 1024; i += 1; } while (n >= 1024 && i < units.length - 1);
    return (n >= 10 ? Math.round(n) : Math.round(n * 10) / 10) + " " + units[i];
  };

  /* KW.coverageDraw draws the coverage details (/api/coverage) into
     body: the namespaces, the kinds, the gaps and the rules to add. */
  KW.coverageDraw = draw;
  KW.coverageDenied = denied;
})();
