/* kwatch dashboard: what the Overview, Activity and kwatch
   pages share: the page head with its freshness line, the refresh loop
   with the shared error panel, and the small dot on the sidebar's
   "kwatch" link that says kwatch itself needs a look. */
(function () {
  "use strict";
  var KW = window.KW;

  /* pageView draws the head of a page into main and refreshes it every
     o.every ms. o.read() reads the page's data and draws it; it returns
     a promise of the main answer ({status}), whose failure shows the
     shared panel (first load) or marks the page old (later). The
     result has body (where the page draws), updated (the freshness
     line, which a page may move), now() and stop(). */
  KW.pageView = function (main, o) {
    if (o.tab) KW.setTitle(o.tab); else KW.title(o.title);
    var head = main.appendChild(KW.el("div", "page-head"));
    var hl = head.appendChild(KW.el("div"));
    hl.appendChild(KW.el("h1", "", o.title));
    if (o.sub) hl.appendChild(KW.el("p", "sub", o.sub));
    var updated = head.appendChild(KW.el("div", "updated"));
    updated.hidden = true;
    var alertBox = main.appendChild(KW.el("div"));
    var body = main.appendChild(KW.el("div", o.cls || ""));
    var lastOk = 0, failing = false, attempt = 0, stopped = false;
    var scrollBack = KW.takeScroll();

    function foot() { KW.footer(updated, lastOk, failing); }

    function load() {
      return Promise.resolve(o.read()).then(function (res) {
        if (stopped || !res) return 0;
        if (KW.authFailed(res)) return 0;
        if (res.status === 200 || res.status === 304) {
          failing = false;
          attempt = 0;
          lastOk = Date.now();
          alertBox.textContent = "";
          body.hidden = false;
          foot();
          // The Issues count in the sidebar stays current on every page.
          KW.refreshBadge();
          if (scrollBack) {
            KW.scrollTo(scrollBack);
            scrollBack = 0;
          }
          return 0;
        }
        failing = true;
        var k = KW.kindOf(res.status);
        if (!lastOk) {
          alertBox.textContent = "";
          body.hidden = true;
          alertBox.appendChild(KW.panel(k, res.status, function () { poll.now(); }));
        }
        foot();
        attempt += 1;
        return KW.retryDelay(k, attempt);
      });
    }
    var poll = KW.poller(load, o.every || 20000, o.wants ? { wants: o.wants } : undefined);
    var tick = window.setInterval(foot, 5000);
    function onConn() {
      foot();
      if (o.conn) o.conn();
    }
    document.addEventListener("kw:conn", onConn);
    return {
      body: body, updated: updated,
      now: function () { poll.now(); },
      lastOk: function () { return lastOk; },
      stop: function () {
        stopped = true;
        poll.stop();
        window.clearInterval(tick);
        document.removeEventListener("kw:conn", onConn);
      }
    };
  };

  /* ---- the "kwatch" link's dot ---- */
  var seen = { status: null, kw: null };

  /* selfIssues: what about kwatch itself needs a look, in plain words,
     from /api/status and its kwatch part (either may be missing). */
  KW.selfIssues = function (s, k) {
    var out = [], self = (s && s.self) || {};
    var failed = (s && s.failedLastHour) || (k && k.failedSendsLastHour) || 0;
    if (failed > 0) out.push(KW.plural(failed, "alert", "alerts") + " failed to send in the last hour");
    if (self.dashboardLinks && self.dashboardLinks.on === false) out.push("alert links are off");
    if (self.startup && !self.startup.done && !(k && k.coverage && k.coverage.syncedAt)) {
      out.push("kwatch is still starting");
    }
    if (self.podLogsForbidden) out.push("kwatch can't read pod logs");
    var c = k && k.coverage;
    if (c && (c.unreadable > 0 || c.unsynced > 0)) out.push("kwatch can't read some kinds of objects");
    if (s && s.lastScanLate) out.push("kwatch hasn't checked the cluster in a while");
    return out;
  };

  /* selfDot takes a fresh /api/status answer and its kwatch part and shows
     or hides the dot. */
  /* providerBanner shows, on every page, each alert channel that is
     failing: what is wrong in plain words, since when, how many alerts
     wait, and where to fix it. It hides when none is. */
  function providerBanner(status) {
    var box = document.querySelector("[data-provbar]");
    if (!box || !status) return;
    var bad = (status.perChannelHealth || []).filter(function (c) { return c.failing; });
    box.hidden = !bad.length;
    box.textContent = "";
    bad.forEach(function (c) {
      var p = box.appendChild(KW.el("p"));
      p.appendChild(KW.el("span", "prov-word", "Failing"));
      var line = "kwatch can't send to " + (c.name || c.channel);
      if (c.since) line += " since " + KW.hm(c.since).textContent;
      line += (c.problem ? " (" + c.problem + ")" : "") + ".";
      if (c.waiting > 0) {
        line += " " + KW.plural(c.waiting, "alert is", "alerts are") + " waiting.";
      }
      line += " Fix: update the settings for " + (c.name || c.channel) +
        " in the installer, under Notifications.";
      p.appendChild(document.createTextNode(line));
    });
  }

  KW.selfDot = function (status, kw) {
    providerBanner(status);
    if (status) seen.status = status;
    if (kw) seen.kw = kw;
    var dot = document.querySelector("[data-kw-dot]");
    if (!dot) return;
    var list = KW.selfIssues(seen.status, seen.kw);
    dot.hidden = !list.length;
    var words = list.length ? "Needs a look: " + list.join("; ") + "." : "";
    dot.title = words;
    var sr = document.querySelector("[data-kw-dot-sr]");
    if (sr) sr.textContent = list.length ? " (needs a look)" : "";
    var a = dot.closest("a");
    if (a) a.title = words || "kwatch itself: is it working and are alerts getting out";
  };

  /* dotPoll reads /api/status every minute on every page, so the dot stays
     right; a page that reads them itself calls KW.selfDot too. */
  var dotPoll = null;
  KW.startDot = function () {
    if (dotPoll) return;
    dotPoll = KW.poller(function () {
      return KW.api("/api/status").then(function (res) {
        var data = res.status === 200 ? res.data : null;
        KW.selfDot(data, data && data.kwatch);
      });
    }, 60000, { wants: function () { return false; } });
  };
  KW.stopDot = function () {
    if (dotPoll) dotPoll.stop();
    dotPoll = null;
  };
})();
