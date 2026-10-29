/* kwatch dashboard: kwatch status. Is kwatch working, and are its
   alerts getting out. */
(function () {
  "use strict";
  var KW = window.KW;
  var EVERY = 20000;
  var DAY = 24 * 3600 * 1000;
  var HINT = "Check the provider settings in the installer: \"Edit notification providers\".";

  function failingNow(c) {
    if (!KW.valid(c.lastError)) return false;
    return !KW.valid(c.lastSuccess) || Date.parse(c.lastError) > Date.parse(c.lastSuccess);
  }

  function stateLine(cls, icon, text) {
    var s = KW.el("span", "ch-state " + cls);
    s.appendChild(KW.icon(icon));
    s.appendChild(KW.el("span", "", text));
    return s;
  }

  function channel(c) {
    var row = KW.el("div", "ch");
    var top = KW.el("div", "ch-top");
    top.appendChild(KW.el("span", "ch-name", c.name || c.channel));
    row.appendChild(top);
    if (failingNow(c)) {
      top.appendChild(stateLine("bad", "x", "last failed " + KW.rel(c.lastError)));
      if (c.lastErrorMessage) {
        var msg = KW.el("p", "ch-msg clamp", c.lastErrorMessage);
        row.appendChild(msg);
        if (c.lastErrorMessage.length > 160) {
          var more = KW.el("button", "link small", "Show more");
          more.type = "button";
          more.addEventListener("click", function () {
            var open = msg.classList.toggle("clamp");
            more.textContent = open ? "Show more" : "Show less";
          });
          row.appendChild(more);
        }
      }
      var detail = KW.valid(c.lastSuccess) ?
        "Last delivered " + KW.rel(c.lastSuccess) + ". " : "It has not delivered yet. ";
      row.appendChild(KW.el("p", "ch-detail", detail + HINT));
      return row;
    }
    if (KW.valid(c.lastSuccess)) {
      top.appendChild(stateLine("ok", "check", "delivering"));
      var d = KW.el("p", "ch-detail", "Last sent " + KW.rel(c.lastSuccess));
      if (KW.valid(c.lastError) && KW.now() - Date.parse(c.lastError) < DAY) {
        d.appendChild(document.createTextNode(" · Failed once " + KW.rel(c.lastError)));
      }
      row.appendChild(d);
      return row;
    }
    top.appendChild(stateLine("none", "dash", "nothing sent yet"));
    return row;
  }

  function pills(names) {
    var box = KW.el("div", "pills");
    names.forEach(function (n) { box.appendChild(KW.el("span", "pill", n)); });
    return box;
  }

  function namespaces(box, s) {
    box.textContent = "";
    var w = s.watchedNamespaces || ["*"];
    var ex = s.excludedNamespaces || [];
    var all = w.length === 1 && w[0] === "*";
    if (all && ex.length) {
      box.appendChild(KW.el("p", "lbl", "Every namespace except:"));
      box.appendChild(pills(ex));
      return;
    }
    if (all) {
      box.appendChild(KW.el("p", "lbl", "Every namespace"));
      return;
    }
    box.appendChild(KW.el("p", "lbl", "Watching"));
    box.appendChild(pills(w));
    if (ex.length) {
      box.appendChild(KW.el("p", "lbl", "Not watching"));
      box.appendChild(pills(ex));
    }
  }

  function summary(node, s) {
    var chans = s.perChannelHealth || [];
    var row;
    if (chans.some(failingNow)) {
      row = ["n-degraded", "kwatch is running, but at least one channel isn't delivering."];
    } else if (!KW.valid(s.lastScan)) {
      row = ["", "kwatch is starting and hasn't finished its first check."];
    } else if (s.lastScanLate) {
      row = ["n-degraded", "kwatch hasn't checked in a while."];
    } else {
      row = ["n-success", "kwatch is running and delivering."];
    }
    node.hidden = false;
    node.className = "notice sum " + row[0];
    KW.$("b", node).textContent = row[1];
  }

  function fill(root, s) {
    summary(KW.$("[data-summary]", root), s);
    var about = KW.$("[data-about]", root);
    about.textContent = "";
    function add(label, value, cls) {
      about.appendChild(KW.el("dt", "", label));
      var dd = KW.el("dd", cls || "");
      if (typeof value === "string") dd.textContent = value;
      else dd.appendChild(value);
      about.appendChild(dd);
    }
    add("Version", s.version || "unknown", "mono");
    var ed = (s.edition || "").toLowerCase();
    add("Plan", ed === "pro" ? "Pro" : ed === "community" ? "Community" : s.edition || "unknown");
    if (KW.valid(s.lastScan)) {
      var last = KW.el("span");
      last.appendChild(KW.timeEl(s.lastScan, "", true));
      last.appendChild(KW.el("br"));
      last.appendChild(KW.el("span", "muted small", KW.full(s.lastScan)));
      add("Last check", last);
    } else {
      add("Last check", "Not yet");
    }
    var ch = KW.$("[data-channels]", root);
    ch.textContent = "";
    var list = s.perChannelHealth || [];
    if (!list.length) {
      ch.appendChild(KW.el("p", "nochan", "No alert channels are set up yet."));
      ch.appendChild(KW.el("p", "muted small",
        "Add one in the installer: choose \"Edit notification providers\"."));
    }
    var wrap = KW.el("div");
    list.forEach(function (c) { wrap.appendChild(channel(c)); });
    ch.appendChild(wrap);
    namespaces(KW.$("[data-namespaces]", root), s);
  }

  KW.statusView = function (main) {
    KW.title("kwatch status");
    main.appendChild(KW.clone("tpl-status"));
    var body = KW.$("[data-body]", main);
    var alertBox = KW.$("[data-alert]", main);
    var footer = KW.$("[data-updated]", main);
    var lastOk = 0, failing = false, attempt = 0, stopped = false;
    var scrollBack = KW.takeScroll();
    function load() {
      KW.refreshBadge();
      return KW.api("/api/status").then(function (res) {
        if (stopped) return 0;
        if (KW.authFailed(res)) return 0;
        alertBox.textContent = "";
        if (res.status !== 200 || !res.data) {
          failing = true;
          var k = KW.kindOf(res.status);
          if (!lastOk || k !== "network") body.hidden = true;
          alertBox.appendChild(KW.panel(k, res.status, function () { poll.now(); }));
          KW.footer(footer, lastOk, true, function () { poll.now(); });
          attempt += 1;
          return KW.retryDelay(k, attempt);
        }
        failing = false;
        attempt = 0;
        lastOk = Date.now();
        KW.self = { namespace: res.data.kwatchNamespace || "kwatch",
          deployment: res.data.kwatchDeployment || "kwatch" };
        body.hidden = false;
        fill(main, res.data);
        KW.footer(footer, lastOk, false);
        if (scrollBack) {
          window.scrollTo(0, scrollBack);
          scrollBack = 0;
        }
        return 0;
      });
    }
    var poll = KW.poller(load, EVERY);
    var tick = window.setInterval(function () {
      if (!failing) KW.footer(footer, lastOk, false);
    }, 5000);
    return { stop: function () { stopped = true; poll.stop(); window.clearInterval(tick); } };
  };
})();
