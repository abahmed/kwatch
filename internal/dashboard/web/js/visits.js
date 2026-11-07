/* kwatch dashboard: "New" and "Changed" since your last visit. This
   browser remembers, per issue, the severity and repeat count it last
   showed you (localStorage key kwatch.visits). An issue that started
   after that, got more urgent, or failed again is marked on the Active
   list until you open it or press "Mark all seen". Without storage
   (private mode) it works for this page only. */
(function () {
  "use strict";
  var KW = window.KW;
  var KEY = "kwatch.visits";
  /* Resolved issues are kept 7 days; forget an issue a day after that. */
  var KEEP = 8 * 24 * 3600 * 1000;
  var MAX = 2000;

  /* ---- what changed: pure functions, no browser state ---- */

  function rank(sev) {
    return KW.RANK[sev] === undefined ? 9 : KW.RANK[sev];
  }

  /* entry is what is remembered of an issue the reader has seen. */
  function entry(item, now) {
    return { severity: item ? item.severity : null,
      repeat: item ? item.repeatCount || 0 : 0, at: now };
  }

  /* marks: id -> {what, was, times} for the listed items. what is
     "new" (started after base and never seen), "escalated" (more
     urgent than when seen) or "again" (failed again since). */
  function marks(store, items) {
    var out = {};
    items.forEach(function (i) {
      var e = store.items[i.id];
      if (!e) {
        if (Date.parse(i.firstSeen) >= store.base - 30000) out[i.id] = { what: "new" };
        return;
      }
      var was = e && typeof e === "object" ? e.severity : null;
      if (was && rank(i.severity) < rank(was)) {
        out[i.id] = { what: "escalated", now: i.severity, was: was };
      }
    });
    items.forEach(function (i) {
      var e = store.items[i.id];
      var more = e ? (i.repeatCount || 0) - e.repeat : 0;
      if (!out[i.id] && more > 0) out[i.id] = { what: "again", times: more };
    });
    return out;
  }

  /* remember updates the store for a fresh list: fills in issues
     opened before the list was loaded, keeps listed ones from being
     forgotten, quietly adds old issues it never saw (so the first
     visit is not all new) and forgets what is long gone. */
  function remember(store, items, now) {
    var seen = {};
    items.forEach(function (i) {
      seen[i.id] = true;
      var e = store.items[i.id];
      if (e && e.severity === null) store.items[i.id] = entry(i, now);
      else if (e) {
        e.at = now;
        // Seen less urgent now: getting worse again is news.
        if (rank(i.severity) > rank(e.severity)) e.severity = i.severity;
      }
      else if (!(Date.parse(i.firstSeen) >= store.base - 30000)) {
        store.items[i.id] = entry(i, now);
      }
    });
    var ids = Object.keys(store.items);
    ids.forEach(function (id) {
      if (!seen[id] && now - store.items[id].at > KEEP) delete store.items[id];
    });
    ids = Object.keys(store.items);
    if (ids.length > MAX) {
      ids.sort(function (a, b) { return store.items[a].at - store.items[b].at; });
      ids.slice(0, ids.length - MAX).forEach(function (id) { delete store.items[id]; });
    }
    return store;
  }

  /* describe: the chip text and its tooltip. */
  function describe(m) {
    if (m.what === "new") {
      return { label: "New", title: "New since your last visit." };
    }
    if (m.what === "escalated") {
      var now = KW.SEVERITY[m.now], was = KW.SEVERITY[m.was];
      return { label: "Changed", title: "Changed since your last visit: " +
        "now " + (now ? now.label : "more urgent") +
        (was ? " (was " + was.label + ")" : "") + "." };
    }
    return { label: "Changed", title: "Changed since your last visit: " +
      (m.times ? "failed again " + KW.plural(m.times, "time", "times") :
        "failed again") + "." };
  }

  /* summary: "2 new, 1 changed since your last visit", or "". */
  function summary(list) {
    var n = 0, c = 0;
    Object.keys(list).forEach(function (id) {
      if (list[id].what === "new") n++; else c++;
    });
    var parts = [];
    if (n) parts.push(n + " new");
    if (c) parts.push(c + " changed");
    return parts.length ? parts.join(", ") + " since your last visit" : "";
  }

  /* ---- this browser ---- */
  function load() {
    var raw = null;
    try { raw = window.localStorage.getItem(KEY); } catch (e) { raw = null; }
    var d = null;
    try { d = raw ? JSON.parse(raw) : null; } catch (e) { d = null; }
    if (!d || typeof d.base !== "number" || !d.items ||
        typeof d.items !== "object") {
      return null;
    }
    // A hand-edited entry that is not an object would break every read.
    Object.keys(d.items).forEach(function (id) {
      var e = d.items[id];
      if (!e || typeof e !== "object") delete d.items[id];
    });
    return d;
  }
  function save() {
    try { window.localStorage.setItem(KEY, JSON.stringify(store)); } catch (e) { /* private mode */ }
  }

  var store = load();
  /* cache: the issues of the list shown last, so opened() can remember
     their severity and repeat count. Only that list is kept. */
  var cache = {};
  /* The previous visit (page load) of this browser, 0 on the first. */
  var lastVisit = store && typeof store.visit === "number" ? store.visit : 0;
  if (store) {
    store.visit = KW.now();
    save();
  }
  function ensure() {
    if (!store) store = { base: KW.now(), visit: KW.now(), items: {} };
    return store;
  }

  KW.visits = {
    /* observe: the marks for an Active list just loaded. */
    observe: function (items) {
      ensure();
      cache = {};
      items.forEach(function (i) { cache[i.id] = i; });
      remember(store, items, KW.now());
      save();
      return marks(store, items);
    },
    /* opened: the reader opened this issue. */
    opened: function (id) {
      ensure().items[id] = entry(cache[id], KW.now());
      save();
    },
    /* markAll: everything listed is seen, and nothing older is new. */
    markAll: function (items) {
      var now = KW.now();
      ensure().base = now;
      items.forEach(function (i) { store.items[i.id] = entry(i, now); });
      save();
    },
    lastVisit: function () { return lastVisit; },
    describe: describe,
    summary: summary
  };
})();
