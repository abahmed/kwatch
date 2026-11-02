/* kwatch dashboard: what every page shares while it is open: the
   active count (nav badge, tab title, favicon), the refresh loop, the
   freshness line and the banner when kwatch can't be reached. main.js
   and the views use them. */
(function () {
  "use strict";
  var KW = window.KW;

  /* ---- active count: nav badge, tab title, favicon ---- */
  KW.badge = null;
  KW.updateBadge = function (counts) {
    KW.badge = counts;
    if (!counts) return;
    var total = 0;
    Object.keys(counts).forEach(function (k) {
      if (k !== "silent") total += counts[k];
    });
    var urgent = (counts.page || 0) > 0;
    var b = document.querySelector("[data-badge]");
    if (b) {
      b.hidden = total === 0;
      b.textContent = KW.num(total);
      b.className = "count" + (urgent ? " urgent" : "");
      b.setAttribute("aria-label", KW.plural(total, "active issue", "active issues"));
    }
    var fav = document.getElementById("favicon");
    if (!fav.dataset.plain) fav.dataset.plain = fav.getAttribute("href");
    fav.setAttribute("href", urgent ? fav.dataset.alert : fav.dataset.plain);
    KW.activeTotal = total;
  };
  var badgeTag = "";
  KW.refreshBadge = function () {
    return KW.api("/api/incidents?status=active&pageSize=25", { etag: badgeTag })
      .then(function (res) {
        if (res.status !== 200 || !res.data || !res.data.facets) return;
        badgeTag = res.etag;
        KW.updateBadge(res.data.facets.severity || {});
      });
  };
  /* setTitle sets the tab title. While the tab is hidden, notify.js
     puts the count of changes not seen yet in front: "(3) ...". */
  KW.baseTitle = "kwatch";
  KW.setTitle = function (text) {
    KW.baseTitle = text;
    var n = KW.unseen || 0;
    document.title = n ? "(" + n + ") " + text.replace(/^\(\d+\) /, "") : text;
  };
  KW.title = function (text) { KW.setTitle(text + " · kwatch"); };

  /* ---- live refresh ---- */
  /* poller runs load now and every `every` ms while the tab is visible,
     at once when it becomes visible again. load returns a promise of a
     delay override (ms) or nothing. While the live stream (below) is
     connected, a poller runs when the stream says incidents changed and
     otherwise only every LIVE_EVERY ms, as a safety net. opts.wants(ids)
     may say whether a change of those incident IDs concerns it. */
  var LIVE_EVERY = 60000;
  var LIVE_GAP = 5000;
  var pollers = [];
  var conn = { lastOk: 0, failing: false, shownSince: Date.now() };
  KW.poller = function (load, every, opts) {
    var timer = 0, stopped = false, last = 0, busy = false, kicked = false;
    var entry = { run: run, kick: kick, wants: opts && opts.wants,
      ago: function () { return Date.now() - last; } };
    function gap() { return Math.max(0, last + LIVE_GAP - Date.now()); }
    /* kick runs it for a live change, at most once per LIVE_GAP ms:
       changes that come faster are read together. */
    function kick() {
      kicked = true;
      if (!busy) schedule(gap());
    }
    function schedule(ms) {
      window.clearTimeout(timer);
      if (!stopped && !document.hidden) timer = window.setTimeout(run, ms);
    }
    /* run loads now; while a load is still out it only asks for one
       more after it, so two loads never race. */
    function run() {
      window.clearTimeout(timer);
      if (stopped) return;
      if (busy) {
        kicked = true;
        return;
      }
      last = Date.now();
      busy = true;
      kicked = false;
      Promise.resolve(load()).then(function (delay) {
        busy = false;
        schedule(delay || (kicked ? gap() : live.on ? LIVE_EVERY : every));
      });
    }
    function onVisible() {
      if (document.hidden) window.clearTimeout(timer);
      else run();
    }
    document.addEventListener("visibilitychange", onVisible);
    pollers.push(entry);
    KW.live.start();
    run();
    return {
      now: run,
      stop: function () {
        stopped = true;
        window.clearTimeout(timer);
        document.removeEventListener("visibilitychange", onVisible);
        pollers = pollers.filter(function (p) { return p !== entry; });
      }
    };
  };

  /* ---- the live stream ---- */
  /* One EventSource per tab on /api/events. "changed" names the
     incidents that changed: the pollers that want them read again
     (each with its ETag, so what didn't change costs a 304). Without
     EventSource, or when the stream fails, is refused or goes silent
     for SILENT ms (a proxy that buffers it), the pollers fall back to
     their own interval. The stream closes while the tab is hidden. */
  var SILENT = 60000;
  var live = { on: false, at: 0, es: null, retry: 0, fails: 0, next: 0,
    started: false, listeners: [] };

  function liveSeen() {
    live.at = Date.now();
    conn.lastOk = live.at;
    conn.failing = false;
    paintBanner();
  }

  function liveOff() {
    if (live.on) {
      live.on = false;
      // Back to polling: each poller's next run uses its own interval.
      KW.tryNow();
    }
  }

  function liveClose() {
    if (live.es) live.es.close();
    live.es = null;
    window.clearTimeout(live.retry);
    live.on = false;
  }

  function liveChanged(e) {
    liveSeen();
    var d = {};
    try { d = JSON.parse(e.data) || {}; } catch (err) { d = {}; }
    var ids = d.ids || [];
    pollers.slice().forEach(function (p) {
      if (d.more || !p.wants || p.wants(ids)) p.kick();
    });
    live.listeners.forEach(function (fn) { fn(ids, !!d.more); });
  }

  function liveOpen() {
    if (live.es || !window.EventSource || document.hidden) return;
    var es = new EventSource("/api/events");
    live.es = es;
    es.addEventListener("ready", function () {
      var was = live.on;
      live.on = true;
      live.fails = 0;
      liveSeen();
      // Whatever was read in the last LIVE_GAP ms is fresh enough.
      if (!was) {
        pollers.slice().forEach(function (p) { if (p.ago() > LIVE_GAP) p.run(); });
      }
    });
    es.addEventListener("ping", liveSeen);
    es.addEventListener("changed", liveChanged);
    // The server's sign-in check failed: show the sign-in page at once and
    // do not reconnect (stop closes the stream and forgets the back-off).
    es.addEventListener("signedout", function () {
      KW.live.stop();
      KW.authFailed({ status: 401 });
    });
    es.onerror = function () {
      liveOff();
      if (es.readyState !== 2) return; // the browser reconnects itself
      live.es = null;
      live.fails += 1;
      live.next = Date.now() + Math.min(live.fails, 10) * 30000;
      liveLater();
    };
  }

  /* liveLater opens the stream again when its back-off ends; showing
     the tab again does not cut the back-off short. */
  function liveLater() {
    window.clearTimeout(live.retry);
    live.retry = window.setTimeout(liveOpen, Math.max(0, live.next - Date.now()));
  }

  KW.live = {
    start: function () {
      if (live.started) return;
      live.started = true;
      liveOpen();
    },
    /* onChange(fn): fn(ids, more) after each change, e.g. for
       notify.js, which keeps its own interval. */
    onChange: function (fn) { live.listeners.push(fn); },
    /* stop closes the stream for good (sign-out, 401) and forgets the
       back-off, so the next start() connects afresh. */
    stop: function () {
      liveClose();
      live.started = false;
      live.fails = 0;
      live.next = 0;
    },
    connected: function () { return live.on; }
  };

  document.addEventListener("visibilitychange", function () {
    if (!live.started) return;
    if (document.hidden) liveClose();
    else liveLater();
  });
  window.setInterval(function () {
    if (live.on && Date.now() - live.at > SILENT) {
      liveClose();
      KW.tryNow();
      liveOpen();
    }
  }, 15000);

  /* ---- the offline / stale banner ---- */
  /* Every API answer passes here. A network failure, a timeout or a 5xx
     marks the data shown as old; any answer that worked (304 too)
     clears it. 401 and 403 are the sign-in flow's, and a request the
     page cancelled itself says nothing. Data older than STALE while the
     tab is shown counts as old too. The banner keeps the page as it is:
     it only says how old it is, with "Try now". */
  var STALE = 45000;
  var api = KW.api;
  KW.api = function (path, opts) {
    return api(path, opts).then(function (res) {
      var s = res.status;
      if ((s >= 200 && s < 400) || s === 404) {
        conn.lastOk = Date.now();
        conn.failing = false;
      } else if ((s === 0 && !(opts && opts.signal && opts.signal.aborted)) || s >= 500) {
        conn.failing = true;
      }
      paintBanner();
      return res;
    });
  };

  /* bannerText: what the banner says, or "" when all is well. */
  function bannerText() {
    var offline = navigator.onLine === false;
    var old = !document.hidden && conn.lastOk &&
      Date.now() - Math.max(conn.lastOk, conn.shownSince) > STALE;
    if (!offline && !conn.failing && !old) return "";
    var from = "";
    if (conn.lastOk) {
      var iso = new Date(conn.lastOk + (KW.now() - Date.now())).toISOString();
      from = " Showing data from " + KW.hhmm(new Date(iso)) + " (" + ago(conn.lastOk) + ").";
    }
    if (offline) return "You're offline." + from;
    return "Can't reach kwatch." + from + " Retrying…";
  }

  /* KW.down: kwatch can't be reached (or the data is old): what the
     page shows is from KW.lastOk() (ms, browser clock), 0 when never.
     A "kw:conn" event on document says it changed. */
  var wasDown = false, foot = null;
  KW.down = function () { return !!bannerText(); };
  KW.lastOk = function () { return conn.lastOk; };

  function paintBanner() {
    var box = document.querySelector("[data-offline]");
    var text = bannerText();
    document.documentElement.classList.toggle("kw-stale", !!text && !!conn.lastOk);
    if (wasDown !== !!text) {
      wasDown = !!text;
      document.dispatchEvent(new Event("kw:conn"));
      // The freshness line says "Offline" at once, not at its next tick.
      if (foot && foot.node.isConnected) KW.footer(foot.node, foot.lastOk, foot.failing);
    }
    if (!box) return;
    box.hidden = !text;
    var line = box.querySelector("[data-offline-text]");
    if (text && line.textContent !== text) line.textContent = text;
  }

  /* tryNow reads everything shown again at once. */
  KW.tryNow = function () { pollers.slice().forEach(function (p) { p.run(); }); };

  document.addEventListener("click", function (e) {
    if (e.target.closest && e.target.closest("[data-offline-try]")) KW.tryNow();
  });
  window.addEventListener("offline", paintBanner);
  window.addEventListener("online", function () { paintBanner(); KW.tryNow(); });
  document.addEventListener("visibilitychange", function () {
    if (!document.hidden) conn.shownSince = Date.now();
    paintBanner();
  });
  window.setInterval(paintBanner, 5000);

  function ago(ms) {
    var s = Math.max(0, Math.round((Date.now() - ms) / 1000));
    return s < 5 ? "just now" : s < 60 ? s + " s ago" : s < 3600 ?
      Math.floor(s / 60) + " min ago" : KW.rel(new Date(ms + KW.skew).toISOString());
  }

  /* footer: the freshness line at the top of a page. "Live · updated
     just now" while the page refreshes by itself; while kwatch can't
     be reached (the view's own read failed, or the banner shows)
     "Offline · last updated 4 min ago", ticking, and never "Live": the banner
     above has "Try now". */
  KW.footer = function (node, lastOk, failing) {
    foot = { node: node, lastOk: lastOk, failing: failing };
    node.hidden = !lastOk;
    if (!lastOk) return;
    failing = failing || KW.down();
    // While the stream is live, nothing has changed since its last word.
    if (!failing && live.on) lastOk = Math.max(lastOk, live.at);
    node.textContent = "";
    node.classList.toggle("stale", !!failing);
    if (failing) {
      var off = KW.el("span", "live", "Offline");
      off.title = "kwatch can't be reached. What you see may be old.";
      node.appendChild(off);
      node.appendChild(KW.el("span", "", "last updated " + ago(lastOk)));
      return;
    }
    var tag = KW.el("span", "live", "Live");
    tag.title = KW.live.connected() ? "Changes arrive as they happen." :
      "This page updates by itself.";
    node.appendChild(tag);
    node.appendChild(KW.el("span", "", "updated " + ago(lastOk)));
  };
})();
