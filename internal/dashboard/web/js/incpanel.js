/* kwatch dashboard: one issue, loaded and kept fresh. The issue page
   (detail.js) and the preview drawer of the Issues table (drawer.js)
   both use it: it reads /api/incidents/<id> every 30 s and draws the
   issue with KW.renderIncident into a tpl-detail clone. While the
   reader has a section open or is selecting text, nothing is redrawn:
   a bar says the issue changed and "Show the update" draws it. */
(function () {
  "use strict";
  var KW = window.KW;
  var EVERY = 30000;

  /* steady is the detail without the parts that change on every read
     (container usage, usage history and the timers of why it is open),
     so a new reading does not redraw the issue. */
  function steady(data) {
    var copy = {};
    Object.keys(data).forEach(function (k) {
      if (k !== "containers" && k !== "usageHours" && k !== "why") copy[k] = data[k];
    });
    var w = data.why;
    if (w) copy.why = [w.alertLevelReason, w.holdTotal, (w.stillOpenReason || "").split(/[,;]/)[0]];
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

  /* incidentPanel shows issue id in box. hooks: page (true on the
     issue page; the drawer leaves out the back link), scrollBack (a
     scroll position to return to once drawn), each() on every read,
     drawn(root, inc) after every draw, notFound() for a 404. */
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

    /* busy: the reader has a section open or is selecting its text. A
       section kwatch opened by itself (data-auto) does not count. */
    function busy() {
      return !!(KW.$("details.fold[open]:not([data-auto]), details[data-keep][open]", root) ||
        KW.selecting(root));
    }

    function draw(data) {
      waiting = null;
      bar.hidden = true;
      lastCore = steady(data);
      if (!root) {
        box.textContent = "";
        box.appendChild(alertBox);
        root = KW.el("div", "d-root");
        root.appendChild(KW.clone("tpl-detail"));
        if (!hooks.page) root.removeChild(KW.$(".d-nav", root));
        KW.$(".detail", root).insertBefore(bar, KW.$(".detail", root).firstChild);
        box.appendChild(root);
      }
      KW.renderIncident(root, data);
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
