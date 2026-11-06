/* kwatch dashboard: what the Issues table shows, as plain data. The
   status, filters, sort and page live in the page address
   (/issues?status=resolved&severity=page), so a view can be bookmarked
   or pasted and opens the same way for anyone. Values the page doesn't
   know are dropped, never sent to the API. Also the quick views. No DOM
   here. */
(function () {
  "use strict";
  var KW = window.KW;

  /* The values each field accepts. The first of each is the default. */
  var STATUSES = ["active", "resolved", "all"];
  /* State picks narrow Active, and on All may add resolved. */
  var STATES = ["open", "settling", "flapping", "recovering", "resolved"];
  var ACKED = ["true", "false"];
  var SINCE = ["", "24h", "72h"];
  var AGES = ["", "today", "day"];
  /* The columns the table sorts by (the API's sort fields). */
  var SORTS = ["", "severity", "title", "namespace", "object", "status",
    "started", "last", "duration", "repeats", "acked"];
  var DIRS = ["", "asc", "desc"];
  var SIZES = [25, 50, 100];
  var MAX_PAGE = 100000;
  var MAX_Q = 200;
  var NAMESPACE = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;
  var KIND = /^[a-z][a-z0-9.]{0,62}$/;
  /* An incident ID, for like= (and open=, see issues.js). */
  var ID = /^[A-Za-z0-9._:-]{1,128}$/;
  /* A node name (node=) and a failure family (mode=, the Overview's
     "Top causes"). */
  var NODE = /^[a-z0-9]([-a-z0-9.]{0,251}[a-z0-9])?$/;
  var MODE = /^[A-Za-z0-9]{1,64}$/;

  function pick(value, allowed) {
    return allowed.indexOf(value || "") >= 0 ? value || "" : "";
  }

  /* blank is the whole active list, in the default order. The
     multi-select filters (severity, namespace, kind, state, acked)
     are lists: values of one are OR-ed, filters are AND-ed. */
  function blank() {
    return { q: "", severity: [], namespace: [], kind: [], state: [],
      acked: [], stopped: [], node: [], mode: [], status: "active", since: "", age: "", sort: "", dir: "",
      page: 1, pageSize: 50, like: "" };
  }

  /* MULTI: the list filters, as they are named in the address. */
  var MULTI = ["severity", "namespace", "kind", "state", "acked", "stopped", "node",
    "mode"];

  /* values reads a repeatable key (namespace=a&namespace=b, or an old
     link's namespace=a,b): the values ok accepts, each once. */
  function values(p, key, ok) {
    var out = [];
    p.getAll(key).forEach(function (v) {
      v.split(",").forEach(function (s) {
        s = s.trim();
        if (s && ok(s) && out.indexOf(s) < 0) out.push(s);
      });
    });
    return out;
  }

  /* stateOK: the states a status allows (resolved only on All). */
  function stateOK(status) {
    return function (v) {
      return STATES.indexOf(v) >= 0 && (v !== "resolved" || status === "all");
    };
  }

  function number(raw, ok, fallback) {
    var n = parseInt(raw || "", 10);
    return ok(n) ? n : fallback;
  }

  /* parse reads the table's state from a query string ("?q=api").
     Old links still open: sort=recent is the "last seen" column, and
     offset and limit become the page and its size; group is ignored. */
  function parse(search) {
    var p = new URLSearchParams(search || "");
    var f = blank();
    f.q = (p.get("q") || "").trim().slice(0, MAX_Q);
    f.severity = values(p, "severity", function (v) { return KW.RANK[v] !== undefined; });
    f.namespace = values(p, "namespace", function (v) {
      return v.length <= 63 && NAMESPACE.test(v);
    });
    f.kind = values(p, "kind", function (v) { return KIND.test(v); });
    f.status = pick(p.get("status"), STATUSES) || "active";
    f.state = f.status === "resolved" ? [] : values(p, "state", stateOK(f.status));
    f.acked = values(p, "acked", function (v) { return ACKED.indexOf(v) >= 0; });
    f.stopped = values(p, "stopped", function (v) { return ACKED.indexOf(v) >= 0; });
    f.node = values(p, "node", function (v) { return NODE.test(v); });
    f.mode = values(p, "mode", function (v) { return MODE.test(v); });
    f.age = pick(p.get("age"), AGES);
    f.like = ID.test(p.get("like") || "") ? p.get("like") : "";
    f.release = p.get("release") === "bad" ? "bad" : "";
    f.since = pick(p.get("since"), SINCE);
    var sort = p.get("sort") === "recent" ? "last" : p.get("sort");
    f.sort = pick(sort, SORTS);
    f.dir = f.sort ? pick(p.get("dir"), DIRS) : "";
    var size = function (n) { return SIZES.indexOf(n) >= 0; };
    f.pageSize = number(p.get("pageSize"), size, number(p.get("limit"), size, 50));
    var offset = number(p.get("offset"), function (n) { return n > 0; }, 0);
    f.page = number(p.get("page"), function (n) { return n >= 1 && n <= MAX_PAGE; },
      Math.min(MAX_PAGE, Math.floor(offset / f.pageSize) + 1));
    return f;
  }

  /* search writes the state back as a query string. Values equal to
     the defaults are left out, except the status, so a pasted link
     opens the same list for anyone. */
  function search(f) {
    var p = new URLSearchParams();
    p.set("status", f.status || "active");
    if (f.q) p.set("q", f.q);
    if (f.like) p.set("like", f.like);
    if (f.release) p.set("release", f.release);
    MULTI.forEach(function (k) {
      if (k === "state" && f.status === "resolved") return;
      f[k].forEach(function (v) { p.append(k, v); });
    });
    if (f.since && f.status !== "active") p.set("since", f.since);
    if (f.age) p.set("age", f.age);
    if (f.sort) p.set("sort", f.sort);
    if (f.sort && f.dir) p.set("dir", f.dir);
    if (f.page > 1) p.set("page", String(f.page));
    if (f.pageSize && f.pageSize !== 50) p.set("pageSize", String(f.pageSize));
    return "?" + p.toString();
  }

  /* used: anything that narrows the list (status, sort and the page
     don't). */
  function used(f) {
    return !!(f.q || f.like || MULTI.some(function (k) { return f[k].length; }) ||
      (f.since && f.status !== "active") || f.age);
  }

  /* sinceMidnight: how long ago midnight today was, in UTC or this
     browser's zone, as an API duration in whole minutes (rounded up, so
     the query only changes once a minute and a poll can still get a
     304). */
  function sinceMidnight(nowMs, utc) {
    var d = new Date(nowMs);
    var ms = utc ? Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate()) :
      new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
    return Math.max(1, Math.ceil((nowMs - ms) / 60000)) + "m";
  }

  /* api is the /api/incidents query of a view. "Started today" follows
     the time zone the dashboard shows. The search text goes through the
     shortcuts of searchq.js: plain words are each a q. */
  function api(f, nowMs, utc) {
    var p = new URLSearchParams();
    p.set("status", f.status);
    KW.searchQuery.toAPI(p, KW.searchQuery.parse(f.q), f);
    if (f.like) p.set("like", f.like);
    if (f.release) p.set("release", f.release);
    f.node.forEach(function (v) { p.append("node", v); });
    f.mode.forEach(function (v) { p.append("mode", v); });
    if (f.since && f.status !== "active") p.set("since", f.since);
    if (f.age === "today") p.set("startedAfter", sinceMidnight(nowMs, utc));
    if (f.age === "day") p.set("startedBefore", "24h");
    if (f.sort) p.set("sort", f.sort);
    if (f.sort && f.dir) p.set("dir", f.dir);
    p.set("page", String(f.page));
    p.set("pageSize", String(f.pageSize));
    return p;
  }

  /* ---- quick views: one click sets the filters (the severity chips
     already show only the urgent ones) ---- */
  var PRESETS = [
    { id: "today", label: "New today",
      title: "Active issues that started today.", set: { age: "today" } },
    { id: "long", label: "Going on over a day",
      title: "Active issues that started more than a day ago.",
      set: { age: "day" } },
    { id: "resolved", label: "Recently resolved",
      title: "Issues resolved in the last 24 hours.",
      set: { status: "resolved", since: "24h" } },
    { id: "stopped", label: "Stopped tracking",
      title: "Resolved issues kwatch stopped tracking while they were still failing.",
      set: { status: "resolved", stopped: ["true"] } }
  ];

  /* presetState: the preset's filters, keeping the order and page size
     of the current view. */
  function presetState(preset, current) {
    var f = blank();
    Object.keys(preset.set).forEach(function (k) {
      f[k] = Array.isArray(preset.set[k]) ? preset.set[k].slice() : preset.set[k];
    });
    if (current) {
      f.pageSize = current.pageSize;
      f.sort = current.sort;
      f.dir = current.dir;
    }
    return f;
  }

  function sameList(a, b) { return a.slice().sort().join() === b.slice().sort().join(); }

  function sameFilters(a, b) {
    return a.q === b.q && a.like === b.like && a.status === b.status && a.since === b.since &&
      a.age === b.age && MULTI.every(function (k) { return sameList(a[k], b[k]); });
  }

  /* copy is f with its lists copied, so a change never reaches f. */
  function copy(f, patch) {
    var out = Object.assign({}, f);
    MULTI.forEach(function (k) { out[k] = f[k].slice(); });
    return Object.assign(out, patch || {});
  }

  /* matchPreset: the id of the quick view shown, or "". */
  function matchPreset(f) {
    for (var i = 0; i < PRESETS.length; i++) {
      if (sameFilters(presetState(PRESETS[i]), f)) return PRESETS[i].id;
    }
    return "";
  }

  KW.listQuery = { blank: blank, parse: parse, search: search, used: used, ID: ID,
    api: api, PRESETS: PRESETS,
    presetState: presetState, matchPreset: matchPreset,
    sameFilters: sameFilters, copy: copy, MULTI: MULTI,
    SIZES: SIZES, STATUSES: STATUSES, SORTS: SORTS };
})();
