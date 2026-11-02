/* kwatch dashboard: "What kwatch is watching" and "kwatch itself" on
   the Overview. "What kwatch is watching" is a
   folded section drawn from /api/coverage: the namespaces watched and
   left out, the kinds of objects watched and fully read, the kinds it
   can't read and why, and when it finished reading the cluster. It
   reads the API with the Overview (an unchanged answer costs a 304),
   since the health line says when kwatch isn't allowed to read some
   kinds; then the section opens by itself, once. "kwatch itself"
   (from /api/status) is a small card of when it started and last
   restarted, plus the notices at the top: still starting, alert links
   off. */
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
    var tot = c.kindTotals || {};
    var unread = (k.unreadable || []);
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
  var STOPPED = {
    graceful_shutdown: "normal restart", leader_handoff: "another kwatch took over",
    internal_failure: "internal error", oom_killed: "ran out of memory",
    eviction: "evicted", node_disruption: "its node went away",
    deployment_rollout: "new version rolled out", api_unavailable: "API server unreachable",
    unknown: "unknown"
  };

  /* size: bytes in words: "812 B", "4.2 MB". */
  function size(n) {
    if (!(n >= 1024)) return Math.max(0, n || 0) + " B";
    var units = ["KB", "MB", "GB"], i = -1;
    do { n /= 1024; i += 1; } while (n >= 1024 && i < units.length - 1);
    return (n >= 10 ? Math.round(n) : Math.round(n * 10) / 10) + " " + units[i];
  }

  /* notice: a box at the top of the Overview. */
  function notice(box, cls, text, role) {
    var n = box.appendChild(KW.el("div", "notice sum " + cls));
    n.appendChild(KW.el("b", "", text));
    if (role) n.setAttribute("role", role);
  }

  /* KW.selfTop fills the top of the Overview: while kwatch is still
     starting (waiting says it hasn't made its first check), and when
     the links in its alerts are off. */
  KW.selfTop = function (box, s, waiting) {
    var self = s.self || {}, st = self.startup, links = self.dashboardLinks;
    box.textContent = "";
    if (st && !st.done) {
      var text = "kwatch is starting: " + num(st.synced) + " of " + num(st.kinds) +
        " kinds read, " + num(st.objects) + " objects";
      if (st.dynamicWatched) text += ", " + num(st.dynamicSynced) + " of " + num(st.dynamicWatched) + " add-on resource types";
      notice(box, "", text + ".", "status");
    } else if (waiting) {
      notice(box, "", "kwatch is starting and hasn't finished its first check.", "status");
    }
    if (links && links.on === false) {
      notice(box, "n-degraded", "Alert links are off: " + (links.text ||
        "alerts can't link to this page."));
    }
    box.hidden = !box.firstChild;
  };

  /* KW.selfCard fills the "kwatch itself" card: only what the API
     sent. Failed sends in the last hour turn warning-colored. */
  KW.selfCard = function (card, s) {
    var self = s.self, dl = KW.$("dl", card);
    card.hidden = !self;
    dl.textContent = "";
    if (!self) return;
    function add(label, value, cls) {
      dl.appendChild(KW.el("dt", "", label));
      var dd = dl.appendChild(KW.el("dd", cls || ""));
      if (value && value.nodeType) dd.appendChild(value);
      else dd.textContent = value;
    }
    if (KW.valid(self.startedAt)) add("Running since", KW.timeEl(self.startedAt));
    var stop = self.lastStop;
    if (stop && KW.valid(stop.at)) {
      var span = KW.el("span");
      span.appendChild(KW.timeEl(stop.at));
      span.appendChild(document.createTextNode(" \u2014 " + (STOPPED[stop.reason] || STOPPED.unknown)));
      add("Last restart", span);
    }
    if (self.restarts) add("Restarts in 24 h", num(self.restarts.length));
    if (KW.valid(self.lastApiContact)) add("Last API contact", KW.timeEl(self.lastApiContact, "", true));
    if (typeof self.outboxPending === "number") add("Messages waiting", num(self.outboxPending));
    if (typeof s.failedLastHour === "number") {
      add("Failed in the last hour", num(s.failedLastHour), s.failedLastHour > 0 ? "x-warn" : "");
    }
    if (typeof self.stateFileBytes === "number") add("State file size", size(self.stateFileBytes));
  };

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
          if (!opened && (denied(res.data) || location.hash === "#cov")) {
            fold.open = true;
            if (location.hash === "#cov") fold.scrollIntoView({ block: "start" });
          }
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
