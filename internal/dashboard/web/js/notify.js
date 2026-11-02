/* kwatch dashboard: browser notifications and the "(3)" in the tab
   title. While a kwatch tab is open (even in the background) it checks
   the issue lists every 30 s and tells you what changed since the last
   check. No service worker and no server push: close every kwatch tab
   and nothing arrives. The Notification API needs no CSP change. */
(function () {
  "use strict";
  var KW = window.KW;
  var EVERY = 30000;
  var KEY = "kwatch.notify";
  /* Over BATCH changes in one check, one summary instead. */
  var BATCH = 3;
  var DEFAULTS = { enabled: false, min: "notify", resolved: false,
    whenOpen: false };

  /* ---- what changed: pure functions, no browser state ---- */

  /* snapshot: id -> {severity, state, title, where, firstSeen} from the
     active and resolved lists. at is when it was taken; complete is
     false when the active list has more than one page. */
  function snapshot(active, resolved, at) {
    var out = { at: at, complete: (active.totalPages || 0) <= 1, items: {} };
    function add(item, state) {
      out.items[item.id] = { id: item.id, severity: item.severity,
        state: state, title: item.title, firstSeen: item.firstSeen,
        where: KW.whereText(item) };
    }
    ((resolved && resolved.incidents) || []).forEach(function (i) {
      add(i, "resolved");
    });
    (active.incidents || []).forEach(function (i) { add(i, "active"); });
    return out;
  }

  function rank(sev) {
    return KW.RANK[sev] === undefined ? 9 : KW.RANK[sev];
  }

  /* changed: what one issue did between two snapshots: "new",
     "reopened", "escalated", "resolved" or "". */
  function changed(before, now, prev) {
    if (now.state === "resolved") {
      return before && before.state === "active" ? "resolved" : "";
    }
    if (!before) {
      // A list cut by its limit can hide an old issue; only one that
      // started since the last check is new then.
      var fresh = prev.complete ||
        Date.parse(now.firstSeen) >= prev.at - EVERY;
      return fresh ? "new" : "";
    }
    if (before.state === "resolved") return "reopened";
    return rank(now.severity) < rank(before.severity) ? "escalated" : "";
  }

  /* changes lists what is worth telling, given the preferences: at
     least prefs.min severe, and resolves only when asked for. */
  function changes(prev, next, prefs) {
    if (!prev) return [];
    var max = rank(prefs.min);
    var out = [];
    Object.keys(next.items).forEach(function (id) {
      var now = next.items[id];
      var what = changed(prev.items[id], now, prev);
      if (!what || rank(now.severity) > max) return;
      if (what === "resolved" && !prefs.resolved) return;
      out.push({ what: what, item: now, was: prev.items[id] });
    });
    return out;
  }

  var WHAT = { "new": "New problem", reopened: "Failing again",
    resolved: "Resolved" };

  /* plan turns changes into notifications: {title, body, tag, url}. */
  function plan(list) {
    if (list.length > BATCH) {
      return [{ title: KW.num(list.length) + " issues changed",
        body: "Open kwatch to see them.", tag: "kwatch-summary",
        url: "/issues" }];
    }
    return list.map(function (c) {
      var sev = KW.SEVERITY[c.item.severity];
      var label = sev ? sev.label : c.item.severity;
      var what = WHAT[c.what] || "Now " + label.toLowerCase() +
        (c.was && KW.SEVERITY[c.was.severity] ?
          " (was " + KW.SEVERITY[c.was.severity].label.toLowerCase() +
          ")" : "");
      return { title: (c.what === "resolved" ? "Resolved" : label) +
          ": " + c.item.title,
        body: c.item.where + " · " + what, tag: c.item.id,
        url: "/issues/" + encodeURIComponent(c.item.id) };
    });
  }

  KW.notifyLogic = { snapshot: snapshot, changes: changes, plan: plan };

  /* ---- preferences: this browser only ---- */
  function load() {
    var p = {};
    try { p = JSON.parse(window.localStorage.getItem(KEY) || "{}") || {}; } catch (e) { p = {}; }
    var out = {};
    Object.keys(DEFAULTS).forEach(function (k) {
      out[k] = typeof p[k] === typeof DEFAULTS[k] ? p[k] : DEFAULTS[k];
    });
    if (KW.RANK[out.min] === undefined || out.min === "silent") out.min = DEFAULTS.min;
    return out;
  }
  function save(p) {
    try { window.localStorage.setItem(KEY, JSON.stringify(p)); } catch (e) { /* private mode */ }
  }

  /* ---- the browser ---- */
  /* blocked says why notifications can't be shown, or "". */
  function blocked() {
    if (!("Notification" in window)) return "This browser can't show notifications.";
    if (window.isSecureContext === false) {
      return "Notifications need a secure (https) address.";
    }
    if (window.Notification.permission === "denied") {
      return "Notifications are blocked for this site. Allow them in " +
        "your browser's site settings, then try again.";
    }
    return "";
  }

  function allowed() {
    return !blocked() && window.Notification.permission === "granted";
  }

  function ask(done) {
    var answered = false;
    function reply(p) {
      if (answered) return;
      answered = true;
      done(p === "granted");
    }
    // Older browsers take a callback instead of returning a promise.
    var r = window.Notification.requestPermission(reply);
    if (r && r.then) r.then(reply, function () { reply("denied"); });
  }

  function show(n) {
    var note;
    try {
      note = new window.Notification(n.title, { body: n.body, tag: n.tag });
    } catch (e) {
      return; // e.g. a browser that only allows them from a service worker
    }
    note.onclick = function () {
      window.focus();
      if (KW.go && location.pathname !== n.url) KW.go(n.url);
      note.close();
    };
  }

  /* ---- the watcher ---- */
  var prefs = load();
  var prev = null, timer = 0, ui = null, busy = false;
  var liveHooked = false, lastLive = 0;
  KW.unseen = 0;

  function looking() {
    return !document.hidden && (!document.hasFocus || document.hasFocus());
  }

  function report(list) {
    if (!list.length) return;
    if (document.hidden) {
      KW.unseen += list.length;
      KW.setTitle(KW.baseTitle);
    }
    if (!prefs.enabled || !allowed()) return;
    if (looking() && !prefs.whenOpen) return;
    plan(list).forEach(show);
  }

  /* The latest activity first, so what just changed is always in the
     page even on a cluster with thousands of issues. Each list is read
     with its ETag: an unchanged list answers 304 and the last copy is
     used again. */
  var LISTS = ["/api/incidents?status=active&sort=last&pageSize=100",
    "/api/incidents?status=resolved&since=24h&sort=last&pageSize=100"];
  var copies = [{ etag: "", data: null }, { etag: "", data: null }];

  function read(i) {
    return KW.api(LISTS[i], { etag: copies[i].etag }).then(function (res) {
      if (res.status === 200 && res.data) copies[i] = { etag: res.etag, data: res.data };
      else if (res.status !== 304) copies[i] = { etag: "", data: null };
      return copies[i].data;
    });
  }

  function check() {
    if (busy) return;
    busy = true;
    Promise.all([read(0), read(1)]).then(function (out) {
      busy = false;
      // One list failed: comparing half a picture would invent changes.
      if (!timer || !out[0] || !out[1]) return;
      var resolved = out[1];
      var next = snapshot(out[0], resolved, KW.now());
      // Never tell about what was already there when the page opened.
      report(changes(prev, next, prefs));
      prev = next;
    });
  }

  function seen() {
    if (document.hidden || !KW.unseen) return;
    KW.unseen = 0;
    KW.setTitle(KW.baseTitle);
  }
  document.addEventListener("visibilitychange", seen);
  window.addEventListener("focus", seen);

  /* ---- the sidebar switch ---- */
  function paint() {
    if (!ui) return;
    var on = prefs.enabled && allowed();
    KW.$("[data-ntf-toggle]", ui).setAttribute("aria-pressed", on ? "true" : "false");
    KW.$("[data-ntf-state]", ui).textContent = on ? "On" : "Off";
    KW.$("[data-ntf-opts]", ui).hidden = !on;
    KW.$("[data-ntf-min]", ui).value = prefs.min;
    KW.$("[data-ntf-resolved]", ui).checked = prefs.resolved;
    KW.$("[data-ntf-open]", ui).checked = prefs.whenOpen;
  }

  function hint(text) {
    var h = KW.$("[data-ntf-hint]", ui);
    h.textContent = text;
    h.hidden = !text;
  }

  function set(key, value) {
    prefs[key] = value;
    save(prefs);
    paint();
  }

  function toggle() {
    hint("");
    if (prefs.enabled && allowed()) return set("enabled", false);
    var why = blocked();
    if (why) return hint(why);
    if (allowed()) return set("enabled", true);
    ask(function (yes) {
      if (yes) return set("enabled", true);
      hint(blocked() || "Your browser didn't allow notifications. " +
        "Try again and choose Allow.");
    });
  }

  function wire(shell) {
    ui = KW.$("[data-ntf]", shell);
    KW.$("[data-ntf-toggle]", ui).addEventListener("click", toggle);
    KW.$("[data-ntf-min]", ui).addEventListener("change", function (e) {
      set("min", e.target.value);
    });
    KW.$("[data-ntf-resolved]", ui).addEventListener("change", function (e) {
      set("resolved", e.target.checked);
    });
    KW.$("[data-ntf-open]", ui).addEventListener("change", function (e) {
      set("whenOpen", e.target.checked);
    });
    // Turned on earlier but no longer allowed: say so.
    if (prefs.enabled && !allowed()) {
      hint(blocked() || "Turn notifications on again to allow them.");
    }
    paint();
  }

  KW.notify = {
    /* start runs with each new shell (after sign-in). */
    start: function (shell) {
      wire(shell);
      if (timer) return;
      prev = null;
      // An interval, not KW.poller: it must keep running while the
      // tab is hidden. Browsers slow it to about once a minute then.
      timer = window.setInterval(check, EVERY);
      check();
      if (!liveHooked) {
        liveHooked = true;
        // A change on the live stream checks at once, at most every
        // 10 s; the interval stays for a hidden tab.
        KW.live.onChange(function () {
          if (timer && Date.now() - lastLive > 10000) {
            lastLive = Date.now();
            check();
          }
        });
      }
    },
    stop: function () {
      window.clearInterval(timer);
      timer = 0;
      prev = null;
      ui = null;
    }
  };
})();
