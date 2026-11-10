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
    KW.blindList(k).forEach(function (b) {
      var h = KW.blindWords(b).head;
      out.push(h.charAt(0).toLowerCase() + h.slice(1));
    });
    return out;
  };

  /* KW.blindWords: a part of kwatch that does not work (the API's
     kwatch.blindSpots), in plain words: head says what kwatch can't do,
     text why and what that means (KW.TERMS.blind). */
  /* KW.cap: a sentence's first letter in capitals, except kwatch's
     own name, which stays lower case. */
  KW.cap = function (t) {
    return /^kwatch\b/.test(t) ? t : t.charAt(0).toUpperCase() + t.slice(1);
  };

  /* KW.blindList: the API's kwatch.blindSpots, then, when issues wait
     for their namespace's labels (kwatch.scopeWaiting), one more. */
  KW.blindList = function (k) {
    var list = ((k && k.blindSpots) || []).slice();
    if (k && k.scopeWaiting > 0) list.push({ part: "scope-waiting", count: k.scopeWaiting });
    return list;
  };

  KW.blindWords = function (b) {
    var B = KW.TERMS.blind, why = B.reasons[b.reason] || B.reasons.other;
    if (b.part === "scope-waiting") {
      return { head: B.scopeWaiting[0].replace("{n}", KW.plural(b.count, "issue is", "issues are")),
        text: B.scopeWaiting[1] };
    }
    var said = KW.cap(why) + ".";
    if (b.part === "license") {
      var l = B.license[b.reason];
      return l ? { head: l[0], text: l[1] } : { head: B.license.other[0], text: said + " " + B.license.other[1] };
    }
    var part = B.parts[b.part];
    if (!part) return { head: B.other.replace("{part}", b.part), text: said };
    return { head: part[0], text: said + " " + part[1] };
  };

  /* KW.cluster: which cluster this dashboard watches, from /api/status:
     name (app.clusterName, "" when unset), label ("unnamed cluster"
     then) and nonProduction. null until signed in and read. Copy
     summary and the tab title read it; the side bar shows it. */
  KW.cluster = null;
  function showCluster(status) {
    var k = status.kwatch || seen.kw;
    var name = status.cluster || "";
    KW.cluster = { name: name, label: name || "unnamed cluster",
      nonProduction: !!(k && k.nonProduction) };
    var box = document.querySelector("[data-cluster]");
    if (box) {
      var n = box.querySelector("[data-cluster-name]");
      n.textContent = KW.cluster.label;
      n.classList.toggle("unnamed", !name);
      box.querySelector("[data-cluster-env]").hidden = !KW.cluster.nonProduction;
      box.hidden = false;
    }
    KW.setTitle(KW.baseTitle);
  }

  /* selfDot takes a fresh /api/status answer and its kwatch part and shows
     or hides the dot. */
  /* KW.providerFix: what to do about a failing channel, by the same
     rule as kwatch's own notice: only a refusal (the API's reason
     provider_rejected) is fixed in the settings; an outage or a rate
     limit passes on its own. */
  KW.providerFix = function (c) {
    if (c.reason === "provider_rejected") {
      return "Fix: update " + (c.name || c.channel) + " in the installer, under Notifications.";
    }
    return "kwatch keeps trying.";
  };

  /* providerBanner shows, on every page, each alert channel that is
     failing: what is wrong in plain words, since when, how many alerts
     wait, and where to fix it. It hides when none is. */
  function providerBanner(status) {
    var box = document.querySelector("[data-provbar]");
    if (!box || !status) return;
    var bad = (status.perChannelHealth || []).filter(function (c) { return c.failing; });
    box.hidden = !bad.length;
    box.textContent = "";
    bad.slice(0, 2).forEach(function (c) {
      var name = c.name || c.channel;
      var p = box.appendChild(KW.el("p"));
      p.appendChild(KW.el("span", "prov-word", "Failing \u00b7"));
      p.appendChild(KW.el("span", "mono", name));
      var line = ": kwatch can't send alerts there";
      if (c.since) line += " since " + KW.hm(c.since).textContent;
      if (c.problem) line += " (" + c.problem + ")";
      line += ".";
      if (c.waiting > 0) {
        line += " " + KW.plural(c.waiting, "alert is", "alerts are") + " waiting.";
      }
      p.appendChild(document.createTextNode(line));
      box.appendChild(KW.el("p", "prov-fix", KW.providerFix(c)));
    });
    if (bad.length > 2) {
      var more = box.appendChild(KW.el("p"));
      var a = KW.el("a", "ulink", "and " + (bad.length - 2) + " more");
      a.href = "/kwatch";
      a.dataset.link = "";
      more.appendChild(a);
    }
  }

  KW.selfDot = function (status, kw) {
    providerBanner(status);
    if (status) seen.status = status;
    if (kw) seen.kw = kw;
    if (status) showCluster(status);
    var dot = document.querySelector("[data-kw-dot]");
    if (!dot) return;
    var list = KW.selfIssues(seen.status, seen.kw);
    dot.hidden = !list.length;
    dot.dataset.count = String(list.length);
    var words = list.length ? "Needs a look (" + KW.num(list.length) + "): " + list.join("; ") + "." : "";
    dot.title = words;
    var sr = document.querySelector("[data-kw-dot-sr]");
    if (sr) sr.textContent = list.length ? " (" + KW.plural(list.length, "thing needs", "things need") + " a look)" : "";
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
