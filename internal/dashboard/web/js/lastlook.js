/* kwatch dashboard: "Changed since you last looked". For each issue
   this browser remembers when it last showed it (localStorage key
   kwatch.looked, at most 200 issues, forgotten after 8 days). On a
   revisit the issue's newer timeline entries, changes and warnings get
   a "New since your last visit" mark and the summary card counts them.
   Also the issue's exact times and IDs ("More details"). */
(function () {
  "use strict";
  var KW = window.KW;
  var KEY = "kwatch.looked", MAX = 200, KEEP = 8 * 24 * 3600 * 1000;

  function load() {
    var d = null;
    try { d = JSON.parse(window.localStorage.getItem(KEY) || "null"); } catch (e) { d = null; }
    return d && typeof d === "object" && !Array.isArray(d) ? d : {};
  }

  /* save keeps the newest MAX entries younger than KEEP. */
  function save(d, now) {
    var ids = Object.keys(d).filter(function (id) {
      return typeof d[id] === "number" && now - d[id] < KEEP;
    }).sort(function (a, b) { return d[b] - d[a]; }).slice(0, MAX);
    var out = {};
    ids.forEach(function (id) { out[id] = d[id]; });
    try { window.localStorage.setItem(KEY, JSON.stringify(out)); } catch (e) { /* private mode */ }
  }

  /* since: when this browser last showed issue id before root was
     first drawn with it (0 on a first visit); it records now. The
     answer is kept on root, so the redraws of one visit agree. */
  function since(root, id) {
    if (root.dataset.lookedId === id) {
      remember(id);
      return Number(root.dataset.lookedAt) || 0;
    }
    var d = load(), prev = typeof d[id] === "number" ? d[id] : 0;
    root.dataset.lookedId = id;
    root.dataset.lookedAt = String(prev);
    remember(id);
    return prev;
  }

  function remember(id) {
    var d = load(), now = KW.now();
    d[id] = now;
    save(d, now);
  }

  var current = 0;

  /* KW.isNew: whether a time is after the reader's last visit of the
     issue being drawn. */
  KW.isNew = function (iso) {
    return current > 0 && KW.valid(iso) && Date.parse(iso) > current;
  };

  /* KW.newMark is the small "New" chip set beside a new entry. */
  KW.newMark = function () {
    return KW.chip("c-new", "New", "New since your last visit");
  };

  function count(inc) {
    var n = 0;
    (inc.timeline || []).forEach(function (e) { if (KW.isNew(e.at)) n++; });
    (inc.transitions || []).forEach(function (t) {
      if (KW.isNew(t.at) && KW.transitionShown(t, inc)) n++;
    });
    (inc.changes || []).forEach(function (c) { if (KW.isNew(c.at)) n++; });
    (inc.warnings || []).forEach(function (w) { if (KW.isNew(w.lastSeen)) n++; });
    return n;
  }

  /* KW.lastLook starts drawing inc into root: it sets what KW.isNew
     compares with and fills the summary line. */
  KW.lastLook = function (root, inc) {
    current = since(root, inc.id);
    var p = KW.$("[data-looked]", root);
    var n = current ? count(inc) : 0;
    p.hidden = !n;
    if (!n) return;
    var at = new Date(current).toISOString();
    p.textContent = (n === 1 ? "1 thing changed" : n + " things changed") +
      " since you last looked (" + KW.when(at) + ")";
    p.title = "Since you last opened this issue in this browser, " + KW.full(at) +
      ". New entries are marked New.";
  };

  /* oneLine: "1 Nov, 18:50" (with UTC when the page shows UTC) on one
     line; the exact time in both zones is in its tooltip. */
  function oneLine(iso) {
    var t = KW.el("time", "d-nowrap", KW.stamp(iso) + (KW.utc ? " UTC" : ""));
    t.setAttribute("datetime", iso);
    t.title = KW.full(iso);
    return t;
  }

  /* KW.facts: the exact times and IDs, under "More details" (Details). */
  KW.facts = function (dl, inc) {
    dl.textContent = "";
    function add(label, value, cls, tip) {
      var dt = dl.appendChild(KW.el("dt", "", label));
      if (tip) dt.title = tip;
      var dd = dl.appendChild(KW.el("dd", cls || ""));
      if (typeof value === "string") dd.textContent = value;
      else dd.appendChild(value);
    }
    var sev = KW.SEVERITY[inc.severity];
    add("Alert level", sev ? sev.level : inc.severity, "", "How kwatch told your team about this.");
    add("Started", oneLine(inc.firstSeen));
    add("Last seen", KW.timeEl(inc.lastSeen));
    if (inc.repeatCount > 0) add("Failed again", KW.plural(inc.repeatCount, "time", "times"));
    if (inc.state === "resolved" && KW.valid(inc.resolvedAt)) {
      add(inc.stoppedTracking ? "Stopped tracking" : "Resolved", KW.stamp(inc.resolvedAt) + " (lasted " +
        KW.lasted(inc.firstSeen, inc.resolvedAt) + ")");
    }
    if (inc.ackedBy) {
      add("Acknowledged", "Yes, on " + inc.ackedBy, "mono",
        "Someone marked this as seen with kwatch's annotation on that object, so kwatch stops reminding.");
    }
    add("Issue ID", inc.id, "mono");
  };

  /* KW.shortNote: one line in the summary when the server shortened
     long text to keep the response small. */
  KW.shortNote = function (root, inc) {
    var p = KW.$("[data-short]", root);
    p.textContent = "Some text shortened";
    p.title = "The server shortened some long text to keep the page small.";
    p.hidden = !KW.cutCount(inc, "texts");
  };
})();
