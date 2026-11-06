/* kwatch dashboard: one issue, loaded and kept fresh, and the tabs of
   the issue page. The issue page (detail.js) and the preview drawer of
   the Issues table (drawer.js) both use it: it reads /api/incidents/<id>
   every 30 s and draws the issue with KW.renderIncident into a
   tpl-detail clone. While the reader has something open in it or is
   selecting text, nothing is redrawn: a bar says the issue changed and
   "Show the update" draws it.
   Tabs (issue page only): Timeline, Evidence, Impact, Alerts, Details,
   each with a count. The last one picked is kept for this browser
   (localStorage kwatch.issueTab); #timeline … #details open one; arrow
   keys move along the tabs and t, e, i, a, d pick one. Under 700px they
   are a select menu. The drawer shows only the summary, with links to
   the tabs of the full page. */
(function () {
  "use strict";
  var KW = window.KW;
  var EVERY = 30000, KEY = "kwatch.issueTab";
  var page = null;

  var EMPTY = {
    timeline: "Nothing happened yet besides kwatch noticing it.",
    evidence: "kwatch has no readings for this yet: no failing probe, logs, containers, warnings or node problems.",
    impact: "Nothing else looks hit: kwatch found no affected objects, lost traffic or linked issues.",
    details: "No more details."
  };

  /* steady is the detail without the parts that change on every read
     (container usage, usage history and the timers of why it is open),
     so a new reading does not redraw the issue. */
  function steady(data) {
    var copy = {};
    Object.keys(data).forEach(function (k) {
      if (k !== "containers" && k !== "usageHours" && k !== "why") copy[k] = data[k];
    });
    var w = data.why;
    if (w) copy.why = [w.alertLevelReason, w.holdTotal, !!w.cannotSee,
      (w.stillOpenReason || "").split(/[,;]/)[0]];
    return JSON.stringify(copy);
  }

  function skeleton(box) {
    var list = KW.el("ul", "list");
    for (var i = 0; i < 3; i++) {
      var li = KW.el("li", "skel");
      li.appendChild(KW.el("span"));
      li.appendChild(KW.el("span"));
      list.appendChild(li);
    }
    list.appendChild(KW.el("li", "sr-only", "Loading"));
    box.appendChild(list);
  }

  function keys() { return KW.ISSUE_TABS.map(function (t) { return t[0]; }); }
  function fromHash() {
    var h = location.hash.slice(1);
    return keys().indexOf(h) >= 0 ? h : "";
  }
  function saved() {
    var v = "";
    try { v = window.localStorage.getItem(KEY) || ""; } catch (e) { v = ""; }
    return keys().indexOf(v) >= 0 ? v : "";
  }
  function save(key) {
    try { window.localStorage.setItem(KEY, key); } catch (e) { /* private mode */ }
  }

  /* select shows tab key. user: the reader picked it, so it is kept and
     put in the address. */
  function select(root, key, user) {
    var bar = KW.$("[data-tabs]", root);
    if (!bar) return;
    KW.ISSUE_TABS.forEach(function (t) {
      var on = t[0] === key, b = KW.$('[data-tab="' + t[0] + '"]', bar);
      b.setAttribute("aria-selected", on ? "true" : "false");
      b.tabIndex = on ? 0 : -1;
      KW.$('[data-panel="' + t[0] + '"]', root).hidden = !on;
    });
    KW.$("[data-tab-pick]", root).value = key;
    root.dataset.shownTab = key;
    if (!user) return;
    save(key);
    if (location.hash !== "#" + key) {
      history.replaceState(history.state, "", location.pathname + location.search + "#" + key);
    }
  }

  /* arrows: Left / Right, Home / End move along the tabs. */
  function arrows(root, e) {
    var list = KW.$$("[role=tab]", root), at = list.indexOf(document.activeElement);
    var to = { ArrowRight: at + 1, ArrowLeft: at - 1, Home: 0, End: list.length - 1 }[e.key];
    if (at < 0 || to === undefined) return;
    e.preventDefault();
    var b = list[(to + list.length) % list.length];
    select(root, b.dataset.tab, true);
    b.focus();
  }

  /* tabBar builds the tabs above the panels of an issue page. */
  function tabBar(root) {
    var nav = KW.el("div", "d-tabbar");
    var list = nav.appendChild(KW.el("div", "d-tabs"));
    list.setAttribute("role", "tablist");
    list.setAttribute("aria-label", "More about this issue");
    list.dataset.tabs = "";
    var label = nav.appendChild(KW.el("label", "d-tabsel"));
    label.appendChild(KW.el("span", "", "Show"));
    var pick = label.appendChild(KW.el("select"));
    pick.name = "issue-tab";
    pick.dataset.tabPick = "";
    KW.ISSUE_TABS.forEach(function (t) {
      var b = list.appendChild(KW.el("button", "d-tab"));
      b.type = "button";
      b.id = "tab-" + t[0];
      b.dataset.tab = t[0];
      b.setAttribute("role", "tab");
      b.setAttribute("aria-controls", "panel-" + t[0]);
      b.setAttribute("aria-keyshortcuts", t[2]);
      b.title = t[1] + " (press " + t[2] + ")";
      b.appendChild(KW.el("span", "", t[1]));
      b.appendChild(KW.el("span", "d-badge")).hidden = true;
      b.addEventListener("click", function () { select(root, t[0], true); });
      pick.appendChild(KW.el("option", "", t[1])).value = t[0];
      var panel = KW.$('[data-panel="' + t[0] + '"]', root);
      panel.id = "panel-" + t[0];
      panel.setAttribute("aria-labelledby", b.id);
    });
    list.addEventListener("keydown", function (e) { arrows(root, e); });
    pick.addEventListener("change", function () { select(root, pick.value, true); });
    var panels = KW.$(".d-panels", root);
    panels.parentNode.insertBefore(nav, panels);
    var old = new URLSearchParams(location.search).get("tab") === "evidence" ? "details" : "";
    select(root, fromHash() || old || saved() || "timeline", false);
  }

  /* nobody: why no channel was told, in plain words. */
  KW.nobodyText = function (inc) {
    if (KW.isUntold(inc)) return "Nobody was told: it settled before alerting.";
    if (inc.severity === "silent") return "Nobody was told: its alert level is Quiet, so kwatch only keeps track of it.";
    if (inc.severity === "digest") return "Nobody was told yet: Low issues wait for the daily summary.";
    if (inc.state === "settling") return "Nobody was told yet: kwatch alerts once it is sure this is real.";
    return "Nobody was notified yet.";
  };

  /* count: the timeline's events; else the sections a tab shows. */
  function count(root, key) {
    if (key === "timeline") return Number(KW.$("[data-timeline]", root).dataset.rows || 0);
    return KW.$$('[data-panel="' + key + '"] > [data-fold]', root).filter(function (s) {
      return !s.hidden;
    }).length;
  }

  /* KW.tabCounts sets each tab's count and empty note, and the drawer's
     links to the full page. */
  KW.tabCounts = function (root, inc) {
    var more = KW.$("[data-ds-more]", root), full = !!KW.$("[data-tabs]", root);
    if (more) more.textContent = "";
    if (more && !full) {
      more.appendChild(KW.el("span", "ds-more-h", "On the full page:"));
      more.hidden = false;
    }
    KW.ISSUE_TABS.forEach(function (t) {
      var n = count(root, t[0]), panel = KW.$('[data-panel="' + t[0] + '"]', root);
      var empty = KW.$(".tab-empty", panel), none = !KW.$$(":scope > [data-fold]", panel).some(function (s) {
        return !s.hidden;
      });
      empty.hidden = !none;
      empty.textContent = t[0] === "alerts" ? KW.nobodyText(inc) : EMPTY[t[0]];
      var words = t[1] + (n ? " (" + KW.num(n) + ")" : "");
      if (full) {
        var b = KW.$('[data-tab="' + t[0] + '"] .d-badge', root);
        b.hidden = !n;
        b.textContent = KW.num(n);
        KW.$('[data-tab-pick] option[value="' + t[0] + '"]', root).textContent = words;
      } else if (more && n) {
        var a = more.appendChild(KW.el("a", "ulink", words));
        a.href = "/issues/" + encodeURIComponent(inc.id) + "#" + t[0];
        a.dataset.link = "";
      }
    });
  };

  /* KW.openFold shows a section: on the issue page its tab, in the
     drawer the full page at that tab. */
  KW.openFold = function (root, name) {
    var sec = KW.$('[data-fold="' + name + '"]', root);
    if (!sec || sec.hidden) return;
    var key = sec.closest("[data-panel]").dataset.panel;
    if (!KW.$("[data-tabs]", root)) {
      KW.go("/issues/" + encodeURIComponent(root.dataset.lookedId || "") + "#" + key);
      return;
    }
    select(root, key, true);
    sec.scrollIntoView({ block: "start" });
  };

  /* Keys t, e, i, a, d pick a tab of the issue page. */
  document.addEventListener("keydown", function (e) {
    if (e.metaKey || e.ctrlKey || e.altKey || e.defaultPrevented || !page || !page.isConnected) return;
    var t = e.target;
    if (t && (t.isContentEditable || /^(INPUT|SELECT|TEXTAREA)$/.test(t.tagName))) return;
    var tab = KW.ISSUE_TABS.filter(function (x) { return x[2] === e.key; })[0];
    if (!tab || document.documentElement.classList.contains("drw-open")) return;
    e.preventDefault();
    select(page, tab[0], true);
    var b = KW.$('[data-tab="' + tab[0] + '"]', page);
    if (b.offsetParent) b.focus({ preventScroll: true });
    KW.$(".d-tabbar", page).scrollIntoView({ block: "nearest" });
  });
  window.addEventListener("hashchange", function () {
    var key = fromHash();
    if (page && page.isConnected && key) { select(page, key, false); save(key); }
  });

  /* incidentPanel shows issue id in box. hooks: page (true on the
     issue page; the drawer leaves out the back link and the tabs),
     scrollBack (a scroll position to return to once drawn), each() on
     every read, drawn(root, inc) after every draw, notFound() for a 404. */
  KW.incidentPanel = function (box, id, hooks) {
    var root = null, attempt = 0, lastCore = "", stopped = false, seq = 0;
    var waiting = null, scrollBack = hooks.scrollBack || 0;
    var alertBox = KW.el("div");
    box.appendChild(alertBox);
    var slow = window.setTimeout(function () { if (!root) skeleton(box); }, 300);
    var bar = KW.el("div", "livebar d-fresh");
    bar.setAttribute("role", "status");
    bar.hidden = true;
    bar.appendChild(KW.el("span", "", "This issue changed."));
    var show = bar.appendChild(KW.el("button", "btn btn-sm", "Show the update"));
    show.type = "button";
    show.addEventListener("click", function () { if (waiting) draw(waiting); });

    /* busy: the reader opened something inside (a diff, all times, how
       to acknowledge) or is selecting text. */
    function busy() {
      return !!(KW.$(".detail details[open]", root) || KW.selecting(root));
    }

    function draw(data) {
      waiting = null;
      bar.hidden = true;
      lastCore = steady(data);
      var first = !root;
      if (first) {
        box.textContent = "";
        box.appendChild(alertBox);
        root = KW.el("div", "d-root");
        root.appendChild(KW.clone("tpl-detail"));
        if (!hooks.page) root.removeChild(KW.$(".d-nav", root));
        KW.$(".detail", root).insertBefore(bar, KW.$(".detail", root).firstChild);
        box.appendChild(root);
      }
      KW.renderIncident(root, data);
      if (first && hooks.page) {
        tabBar(root);
        page = root;
      }
      if (first && !hooks.page) KW.$(".d-panels", root).hidden = true;
      KW.tabCounts(root, data);
      if (hooks.drawn) hooks.drawn(root, data);
      if (scrollBack) {
        window.scrollTo(0, scrollBack);
        scrollBack = 0;
      }
    }

    /* failed: an issue already drawn stays, and the banner of
       refresh.js says it may be old; with nothing drawn yet, a panel
       says what went wrong. */
    function failed(res) {
      var k = KW.kindOf(res.status);
      if (!root) {
        box.textContent = "";
        box.appendChild(alertBox);
        alertBox.appendChild(KW.panel(k, res.status, function () { poll.now(); }));
      }
      attempt += 1;
      return KW.retryDelay(k, attempt);
    }

    function load() {
      if (hooks.each) hooks.each();
      var mine = ++seq;
      return KW.api("/api/incidents/" + encodeURIComponent(id)).then(function (res) {
        // An answer that arrives after the issue was left, or after a
        // newer read was asked for, changes nothing.
        if (stopped || mine !== seq) return 0;
        window.clearTimeout(slow);
        if (KW.authFailed(res)) return 0;
        if (res.status === 404) {
          poll.stop();
          hooks.notFound();
          return 0;
        }
        alertBox.textContent = "";
        if (res.status !== 200 || !res.data) return failed(res);
        attempt = 0;
        if (root && steady(res.data) === lastCore) {
          waiting = null;
          bar.hidden = true;
          if (!busy()) {
            KW.liveObjects(root, res.data);
            KW.usageChart(root, res.data);
            KW.tabCounts(root, res.data);
          }
          KW.alertBox(root, res.data, true);
          return 0;
        }
        if (root && busy()) {
          waiting = res.data;
          bar.hidden = false;
          return 0;
        }
        draw(res.data);
        return 0;
      });
    }
    var poll = KW.poller(load, EVERY, {
      wants: function (ids) { return ids.indexOf(id) >= 0; }
    });
    return { stop: function () { stopped = true; poll.stop(); window.clearTimeout(slow); } };
  };
})();
