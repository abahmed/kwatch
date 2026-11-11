/* kwatch dashboard: browser notifications and the "(3)" in the tab
   title. Where the browser can, kwatch pushes each change itself
   (push.js), even with every kwatch tab closed. Otherwise an open tab
   (even in the background) checks the issue lists every 30 s and
   shows what changed since the last check. Either way the rules are
   the same (push_changes.go follows these). */
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
     false when the active list has more than one page. An issue still
     settling, or one no message reached, is left out (and only noted
     in waiting): it is news once it is confirmed, and one that settles
     quietly is never told, neither new nor resolved. */
  function snapshot(active, resolved, at) {
    var out = { at: at, complete: (active.totalPages || 0) <= 1, items: {},
      waiting: {} };
    function add(item, state) {
      if (item.state === "settling" || item.untold) {
        out.waiting[item.id] = true;
        return;
      }
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
      // One that was settling then is new now that it is confirmed.
      var fresh = prev.complete || (prev.waiting || {})[now.id] ||
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
      return "Browser alerts need a secure (https) address.";
    }
    if (window.Notification.permission === "denied") {
      return "Browser alerts are blocked for this site. Allow them in " +
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
    // kwatch pushes them itself; this tab only counts.
    if (!prefs.enabled || !allowed() || KW.push.active()) return;
    if (looking() && !prefs.whenOpen) return;
    plan(list).forEach(KW.push.show);
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

  var checkFailed = false;

  /* check reads the two lists and tells what changed. With browser
     alerts off it reads nothing (and forgets the last picture, so
     turning them on later never tells what changed meanwhile). While
     the page is in view and the live stream is up, only the stream's
     changes run it (fromLive): the 30 s interval would read the same
     lists again. A hidden tab keeps the interval: that is when alerts
     matter. */
  function check(fromLive) {
    if (!prefs.enabled || !allowed()) {
      prev = null;
      return;
    }
    if (fromLive !== true && prev && !document.hidden && KW.live.connected()) return;
    if (busy) return;
    busy = true;
    var run;
    try { run = Promise.all([read(0), read(1)]); } catch (e) { run = Promise.reject(e); }
    run.then(function (out) {
      busy = false;
      // One list failed: comparing half a picture would invent changes.
      if (!timer || !out[0] || !out[1]) return;
      var resolved = out[1];
      var next = snapshot(out[0], resolved, KW.now());
      // Never tell about what was already there when the page opened.
      report(changes(prev, next, prefs));
      KW.push.close(ended(prev, next));
      prev = next;
      KW.push.check().then(function (lost) {
        if (lost === "lost" && prefs.enabled && allowed()) connect(true);
        paint();
      });
    }).then(null, function (err) {
      // The next check runs on the timer; one bad read must not stop it.
      busy = false;
      if (!checkFailed) console.error("kwatch: notification check failed", err);
      checkFailed = true;
    });
  }

  /* ended: the issues that resolved since the last check, whose
     notifications are done with. */
  function ended(before, now) {
    if (!before) return [];
    return Object.keys(now.items).filter(function (id) {
      return now.items[id].state === "resolved" && before.items[id] &&
        before.items[id].state === "active";
    });
  }

  /* connect has kwatch push to this browser, or keeps the tab
     showing them when it can't. */
  function connect(fresh) {
    KW.push.subscribe(prefs, fresh).then(paint);
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
    // Pushed ones arrive whether or not a tab is open.
    KW.$("[data-ntf-open-l]", ui).hidden = KW.push.on();
    var failing = KW.push.failing();
    KW.$("[data-ntf-mode]", ui).textContent = !KW.push.on() ?
      "They arrive while a kwatch tab is open." : failing ?
      failing + " Until then they arrive while a kwatch tab is open." :
      "They arrive even when kwatch is closed.";
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
    // kwatch decides what to push, so it hears of every change.
    if (key !== "enabled" && key !== "whenOpen" && KW.push.on()) {
      connect();
    }
  }

  function turnOn() {
    set("enabled", true);
    connect();
    // The first read only takes the picture to compare with.
    if (timer) check(false);
  }

  /* test sends one notification the way real ones come. */
  function test() {
    hint("");
    if (!KW.push.on()) {
      KW.push.show({ title: "kwatch notifications work",
        body: "You'll get one like this when an issue needs you.",
        tag: "kwatch-test", url: "/" });
      return;
    }
    KW.push.test().then(function (why) {
      if (why) return hint(why);
      if (ui) {
        KW.$("[data-ntf-mode]", ui).textContent =
          "Sent. It should appear in a few seconds.";
      }
    });
  }

  function toggle() {
    hint("");
    if (prefs.enabled && allowed()) {
      KW.push.unsubscribe().then(paint);
      return set("enabled", false);
    }
    var why = blocked();
    if (why) return hint(why);
    if (allowed()) return turnOn();
    ask(function (yes) {
      if (yes) return turnOn();
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
    KW.$("[data-ntf-test]", ui).addEventListener("click", test);
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
      // Subscribing again keeps this browser known to kwatch.
      if (prefs.enabled && allowed()) connect();
      if (timer) return;
      prev = null;
      // An interval, not KW.poller: it must keep running while the
      // tab is hidden. Browsers slow it to about once a minute then.
      timer = window.setInterval(function () { check(false); }, EVERY);
      check(false);
      if (!liveHooked) {
        liveHooked = true;
        // A change on the live stream checks at once, at most every
        // 10 s; the interval stays for a hidden tab.
        KW.live.onChange(function () {
          if (timer && Date.now() - lastLive > 10000) {
            lastLive = Date.now();
            check(true);
          }
        });
      }
    },
    /* opened: the issue is open here; its notification is done. */
    opened: function (id) { KW.push.close([id]); },
    /* signOut stops pushes to this browser before the session ends. */
    signOut: function () { return KW.push.unsubscribe(); },
    /* signedOut: the sign-in ended without Sign out (it expired);
       pushes to this browser stop until it signs in again. */
    signedOut: function () { KW.push.forget(); },
    stop: function () {
      window.clearInterval(timer);
      timer = 0;
      prev = null;
      ui = null;
    }
  };
})();
