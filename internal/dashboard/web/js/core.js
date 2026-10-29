/* kwatch dashboard: shared helpers. Plain browser JavaScript, no build
   step. API text is always set with textContent, never parsed as HTML. */
(function () {
  "use strict";
  var KW = (window.KW = {});
  var SVG = "http://www.w3.org/2000/svg";

  KW.$ = function (sel, root) { return (root || document).querySelector(sel); };
  KW.$$ = function (sel, root) {
    return Array.prototype.slice.call((root || document).querySelectorAll(sel));
  };

  /* el builds an element with a class and text. */
  KW.el = function (tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined && text !== null) e.textContent = text;
    return e;
  };

  KW.clone = function (id) {
    return document.getElementById(id).content.cloneNode(true);
  };

  KW.icon = function (name) {
    var svg = document.createElementNS(SVG, "svg");
    svg.setAttribute("class", "icon");
    svg.setAttribute("aria-hidden", "true");
    var use = document.createElementNS(SVG, "use");
    use.setAttribute("href", "#i-" + name);
    svg.appendChild(use);
    return svg;
  };

  KW.announce = function (text) {
    var region = document.getElementById("announce");
    region.textContent = "";
    window.setTimeout(function () { region.textContent = text; }, 50);
  };

  /* ---- time ---- */
  var skew = 0;
  KW.now = function () { return Date.now() + skew; };
  var MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug",
    "Sep", "Oct", "Nov", "Dec"];
  function pad(n) { return n < 10 ? "0" + n : "" + n; }

  /* Clock times show in local time or UTC; on-call teams often talk in
     UTC. The choice is remembered on this browser only. */
  KW.utc = false;
  try { KW.utc = window.localStorage.getItem("kwatch.tz") === "utc"; } catch (e) { KW.utc = false; }
  KW.setUTC = function (on) {
    KW.utc = !!on;
    try { window.localStorage.setItem("kwatch.tz", on ? "utc" : "local"); } catch (e) { /* private mode */ }
  };
  function H(d) { return KW.utc ? d.getUTCHours() : d.getHours(); }
  function M(d) { return KW.utc ? d.getUTCMinutes() : d.getMinutes(); }
  function D(d) { return KW.utc ? d.getUTCDate() : d.getDate(); }
  function Mo(d) { return KW.utc ? d.getUTCMonth() : d.getMonth(); }
  function Y(d) { return KW.utc ? d.getUTCFullYear() : d.getFullYear(); }
  KW.hhmm = function (d) { return pad(H(d)) + ":" + pad(M(d)); };
  KW.dayMonth = function (d) { return D(d) + " " + MONTHS[Mo(d)]; };
  function sameDay(a, b) {
    return Y(a) === Y(b) && Mo(a) === Mo(b) && D(a) === D(b);
  }
  KW.isToday = function (d) { return sameDay(d, new Date(KW.now())); };
  KW.isYesterday = function (d) {
    return sameDay(d, new Date(KW.now() - 24 * 3600 * 1000));
  };
  KW.valid = function (iso) {
    return !!iso && !isNaN(Date.parse(iso)) && Date.parse(iso) > 0;
  };

  /* rel: "just now", "4 min ago", "3 h ago", "yesterday at 14:02",
     "12 Oct, 14:02". seconds: "20 seconds ago" under a minute. */
  KW.rel = function (iso, seconds) {
    var d = new Date(iso);
    var s = Math.max(0, Math.round((KW.now() - d.getTime()) / 1000));
    if (s < 60) {
      if (!seconds || s < 5) return "just now";
      return s + " seconds ago";
    }
    if (s < 3600) return Math.floor(s / 60) + " min ago";
    if (KW.isToday(d)) return Math.floor(s / 3600) + " h ago";
    if (KW.isYesterday(d)) return "yesterday at " + KW.hhmm(d);
    return KW.dayMonth(d) + ", " + KW.hhmm(d);
  };

  /* full: the exact time in both zones, each with its date (they can
     fall on different days), the zone shown on the page first. */
  KW.full = function (iso) {
    var d = new Date(iso);
    var utc = d.getUTCDate() + " " + MONTHS[d.getUTCMonth()] + ", " +
      pad(d.getUTCHours()) + ":" + pad(d.getUTCMinutes()) + ":" + pad(d.getUTCSeconds()) + " UTC";
    var local = d.getDate() + " " + MONTHS[d.getMonth()] + ", " + pad(d.getHours()) + ":" +
      pad(d.getMinutes()) + ":" + pad(d.getSeconds()) + " local";
    return KW.utc ? utc + " · " + local : local + " · " + utc;
  };

  /* takeScroll returns, once, a scroll position a redraw should go back
     to (after a time zone switch), or 0. */
  KW.pendingScroll = 0;
  KW.takeScroll = function () {
    var y = KW.pendingScroll;
    KW.pendingScroll = 0;
    return y;
  };

  KW.stamp = function (iso) {
    var d = new Date(iso);
    return KW.dayMonth(d) + ", " + KW.hhmm(d);
  };

  /* when: a clock time that says which day when it isn't today:
     "14:02", "yesterday 14:02", "12 Oct, 14:02". */
  KW.when = function (iso) {
    var d = new Date(iso);
    if (KW.isToday(d)) return KW.hhmm(d);
    if (KW.isYesterday(d)) return "yesterday " + KW.hhmm(d);
    return KW.stamp(iso);
  };

  /* lasted: "12 min", "3 h 10 min", "2 days". */
  KW.lasted = function (fromIso, toIso) {
    var m = Math.max(1, Math.round((Date.parse(toIso) - Date.parse(fromIso)) / 60000));
    if (m < 60) return m + " min";
    if (m < 1440) {
      var h = Math.floor(m / 60), rest = m % 60;
      return h + " h" + (rest ? " " + rest + " min" : "");
    }
    var days = Math.round(m / 1440);
    return days + (days === 1 ? " day" : " days");
  };

  /* timeEl is a <time> that the 30 s ticker keeps relative. */
  KW.timeEl = function (iso, prefix, seconds) {
    var t = KW.el("time");
    t.setAttribute("datetime", iso);
    t.setAttribute("title", KW.full(iso));
    t.dataset.rel = iso;
    t.dataset.prefix = prefix || "";
    if (seconds) t.dataset.seconds = "1";
    t.textContent = (prefix || "") + KW.rel(iso, seconds);
    return t;
  };
  KW.tickTimes = function () {
    KW.$$("time[data-rel]").forEach(function (t) {
      t.textContent = t.dataset.prefix + KW.rel(t.dataset.rel, !!t.dataset.seconds);
    });
  };

  KW.plural = function (n, one, many) { return n + " " + (n === 1 ? one : many); };

  /* ---- words ---- */
  KW.SEVERITY = {
    page: { label: "Urgent", chip: "c-page", stripe: "sev-page",
      title: "kwatch sent this as a page. Someone should look now." },
    notify: { label: "Needs attention", chip: "c-notify", stripe: "sev-notify",
      title: "kwatch notified your channels. Look at it soon." },
    digest: { label: "Low", chip: "c-low", stripe: "sev-low",
      title: "Not urgent. It goes into the digest, not a page." },
    silent: { label: "Quiet", chip: "c-quiet", stripe: "sev-low",
      title: "kwatch is tracking this but is not sending it." }
  };
  KW.STATE = {
    settling: { label: "Looking into it", chip: "c-state",
      title: "kwatch is still collecting what failed around this, so the cause may change." },
    open: { label: "Happening now", chip: "c-state",
      title: "This is failing right now." },
    recovering: { label: "Looks better", chip: "c-state c-better",
      title: "It stopped failing. kwatch is waiting a little to be sure." },
    flapping: { label: "Keeps coming back", chip: "c-state c-flap",
      title: "It keeps failing and recovering." },
    resolved: { label: "Resolved", chip: "c-resolved",
      title: "It stopped failing and stayed fixed." }
  };

  KW.chip = function (cls, text, title) {
    var c = KW.el("span", "chip " + cls, text);
    if (title) c.title = title;
    return c;
  };
  KW.severityChip = function (sev, prefix, extra) {
    var s = KW.SEVERITY[sev];
    if (!s) return KW.chip("c-low c-raw" + (extra || ""), (prefix || "") + sev);
    return KW.chip(s.chip + (extra || ""), (prefix || "") + s.label, s.title);
  };
  KW.stateChip = function (state) {
    var s = KW.STATE[state];
    if (!s) return KW.chip("c-state c-raw", state || "unknown");
    return KW.chip(s.chip, s.label, s.title);
  };

  /* where: namespace / workload. stacked puts them on two lines, so a
     long workload name wraps on its own line instead of mid-word. */
  KW.where = function (item, node, stacked) {
    if (!item.namespace) {
      node.classList.add("cluster");
      node.textContent = "whole cluster";
      if (item.workload) node.textContent += " · " + item.workload;
      return;
    }
    if (stacked && item.workload) {
      node.textContent = "";
      node.appendChild(KW.el("span", "w-ns", item.namespace));
      node.appendChild(KW.el("span", "w-wl", item.workload));
      return;
    }
    node.textContent = item.namespace + (item.workload ? " / " + item.workload : "");
  };

  KW.failedAgain = function (n) {
    return "Failed again " + KW.plural(n, "time", "times");
  };

  /* ---- API ---- */
  /* api returns {status, data, retryAfter}; status 0 is a network
     failure. The Date header corrects the browser clock. */
  KW.api = function (path, opts) {
    opts = opts || {};
    var init = { method: opts.method || "GET", credentials: "same-origin",
      cache: "no-store", headers: { Accept: "application/json" } };
    if (opts.body !== undefined) {
      init.headers["Content-Type"] = "application/json";
      init.body = JSON.stringify(opts.body);
    }
    var ctl = window.AbortController ? new AbortController() : null;
    if (ctl) {
      init.signal = ctl.signal;
      window.setTimeout(function () { ctl.abort(); }, 15000);
    }
    return fetch(path, init).then(function (res) {
      var date = Date.parse(res.headers.get("Date") || "");
      if (!isNaN(date)) skew = date - Date.now();
      var retry = parseInt(res.headers.get("Retry-After") || "", 10);
      return res.text().then(function (text) {
        var data = null;
        try { data = text ? JSON.parse(text) : null; } catch (e) { data = null; }
        return { status: res.status, data: data, retryAfter: isNaN(retry) ? 0 : retry };
      });
    }, function () {
      return { status: 0, data: null, retryAfter: 0 };
    });
  };

  /* ---- copy ---- */
  KW.copy = function (button, text, label) {
    function done() {
      button.textContent = "Copied";
      KW.announce("Copied");
      window.setTimeout(function () { button.textContent = label || "Copy"; }, 2000);
    }
    function fallback() {
      var target = button.parentNode.querySelector("code, .logs");
      if (!target) return;
      var range = document.createRange();
      range.selectNodeContents(target);
      var sel = window.getSelection();
      sel.removeAllRanges();
      sel.addRange(range);
    }
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(done, fallback);
    } else {
      fallback();
    }
  };

  /* cmd is a command box: one line, scrolls inside, Copy copies it. */
  KW.cmd = function (command) {
    var box = KW.el("div", "cmd");
    var code = KW.el("code", "", command);
    code.tabIndex = 0;
    var b = KW.el("button", "btn", "Copy");
    b.type = "button";
    b.addEventListener("click", function () { KW.copy(b, command); });
    box.appendChild(code);
    box.appendChild(b);
    return box;
  };

  KW.wireCopy = function (root) {
    KW.$$("[data-copy]", root).forEach(function (b) {
      b.addEventListener("click", function () {
        KW.copy(b, b.parentNode.querySelector("code").textContent);
      });
    });
  };

  /* ---- shared states ---- */
  KW.self = { namespace: "kwatch", deployment: "kwatch" };

  /* panel renders one of the shared error or starting panels.
     kind: starting | network | limit | error. */
  KW.panel = function (kind, status, onRetry) {
    var frag = KW.clone("tpl-panel");
    var p = frag.firstElementChild;
    var retry = KW.$("[data-retry]", p);
    var texts = {
      starting: ["kwatch is starting",
        "kwatch has just started and is still loading what it knows. This page refreshes by itself.",
        "Try now"],
      network: ["Can't reach kwatch", "The connection dropped. We'll keep trying.", "Try now"],
      limit: ["Too many tries", "Wait a minute, then try again.", ""],
      error: ["Something went wrong inside kwatch",
        "This is not your cluster. Check kwatch's own logs for the reason:", "Try again"]
    }[kind];
    KW.$("[data-heading]", p).textContent = texts[0];
    KW.$("[data-body]", p).textContent = texts[1];
    if (kind === "error") {
      var cmd = KW.$("[data-cmd]", p);
      cmd.hidden = false;
      KW.$("code", cmd).textContent = "kubectl -n " + KW.self.namespace +
        " logs deploy/" + KW.self.deployment + " --tail=50";
      KW.wireCopy(p);
      var code = KW.$("[data-code]", p);
      code.hidden = false;
      code.textContent = "Error " + status;
    }
    if (texts[2] && onRetry) {
      retry.hidden = false;
      retry.textContent = texts[2];
      retry.addEventListener("click", onRetry);
    }
    return p;
  };

  /* kindOf sorts a failed API status into a shared state. */
  KW.kindOf = function (status) {
    if (status === 0) return "network";
    if (status === 503) return "starting";
    if (status === 429) return "limit";
    return "error";
  };

  /* retryDelay: 5 s for the first tries while starting, then 15 s;
     60 s after 429. */
  KW.retryDelay = function (kind, attempt) {
    if (kind === "limit") return 60000;
    if (kind === "starting") return attempt < 6 ? 5000 : 15000;
    return 15000;
  };
})();
