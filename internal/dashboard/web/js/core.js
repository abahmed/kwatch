/* kwatch dashboard: shared helpers. Plain browser JavaScript, no build
   step. API text is always set with textContent, never parsed as HTML. */
(function () {
  "use strict";
  /* look.js, loaded first, already put KW.look here. */
  var KW = (window.KW = window.KW || {});
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
  /* skew corrects this browser's clock by the server's (api.js). */
  KW.skew = 0;
  KW.now = function () { return Date.now() + KW.skew; };
  var MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug",
    "Sep", "Oct", "Nov", "Dec"];
  function pad(n) { return n < 10 ? "0" + n : "" + n; }

  /* KW.store reads and writes this browser's view options (localStorage
     key kwatch.view: time zone, columns, page size, saved views...).
     Every access may throw (private mode, blocked storage); then the
     options last until the page is reloaded. */
  var VIEW = "kwatch.view", viewMemo = null;
  KW.store = {
    all: function () {
      if (viewMemo) return viewMemo;
      var v = null;
      try { v = JSON.parse(window.localStorage.getItem(VIEW) || "null"); } catch (e) { v = null; }
      viewMemo = v && typeof v === "object" && !Array.isArray(v) ? v : {};
      return viewMemo;
    },
    get: function (k) { return KW.store.all()[k]; },
    set: function (k, value) {
      var v = KW.store.all();
      if (value === undefined) delete v[k]; else v[k] = value;
      try { window.localStorage.setItem(VIEW, JSON.stringify(v)); } catch (e) { /* private mode */ }
    }
  };

  /* Clock times show in local time or UTC; on-call teams often talk in
     UTC. The choice is a view option of this browser (the old key
     kwatch.tz is still read). */
  KW.utc = KW.store.get("tz") === "utc";
  if (KW.store.get("tz") === undefined) {
    try { KW.utc = window.localStorage.getItem("kwatch.tz") === "utc"; } catch (e) { KW.utc = false; }
  }
  KW.setUTC = function (on) {
    KW.utc = !!on;
    KW.store.set("tz", on ? "utc" : "local");
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

  /* hm: a <time> with the clock time ("14:02"; "yesterday 14:02" or
     "12 Oct, 14:02" when it isn't today) and the exact time, seconds
     included, in its tooltip. The one clock format of the dashboard. */
  KW.hm = function (iso) {
    var t = KW.el("time", "", KW.when(iso));
    t.setAttribute("datetime", iso);
    t.title = KW.full(iso);
    return t;
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

  /* num: a count with thousands separators: "4,812". The one number
     format of the dashboard. */
  KW.num = function (n) { return Number(n || 0).toLocaleString("en-US"); };
  KW.plural = function (n, one, many) {
    return KW.num(n) + " " + (n === 1 ? one : many);
  };

  /* obj: an object as "deployment/payments-api" (the kind in lower
     case). The namespace is never part of it: it is shown on its own
     ("payments", or "whole cluster" without one). With no name the
     object is the namespace itself, or the whole cluster: "". */
  KW.obj = function (kind, ns, name) {
    if (!name) return "";
    return (kind ? String(kind).toLowerCase() + "/" : "") + name;
  };
  /* KW.cutCount: how many entries of a part the server left out to keep
     the response within its size limit (inc.truncated), else 0. */
  KW.cutCount = function (inc, part) {
    return (inc.truncated && inc.truncated[part]) || 0;
  };

  /* ---- words ---- */
  /* Severity is kwatch's alert level (the API's severity: page,
     notify, digest, silent). label is the badge; level says how the
     team was told, in plain words, for tooltips and the legend. */
  KW.SEVERITY = {
    page: { label: "Urgent", chip: "c-page", stripe: "sev-page",
      level: "Alerted the on-call person",
      title: "Alert level: alerted the on-call person. Look now." },
    notify: { label: "Needs attention", chip: "c-notify", stripe: "sev-notify",
      level: "Sent a message",
      title: "Alert level: sent a message to your channels. Look soon." },
    digest: { label: "Low", chip: "c-low", stripe: "sev-low",
      level: "In the daily summary",
      title: "Alert level: in the daily summary only. No rush." },
    silent: { label: "Quiet", chip: "c-quiet", stripe: "sev-low",
      level: "Not sent",
      title: "Alert level: not sent. kwatch only keeps track of it." }
  };
  /* RANK orders severities, most urgent first. */
  KW.RANK = { page: 0, notify: 1, digest: 2, silent: 3 };
  KW.STATE = {
    settling: { label: "Confirming", chip: "c-state",
      title: "kwatch is checking before it alerts." },
    open: { label: "Happening now", chip: "c-state c-open",
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

  /* whereText: "payments · deployment/api" or "whole cluster ·
     node/ip-10-0-4-7": the namespace, then the object (KW.obj). */
  KW.whereText = function (item) {
    var o = KW.obj(item.workloadKind || item.kind, item.namespace,
      item.workload);
    return (item.namespace || "whole cluster") + (o ? " · " + o : "");
  };

  /* where puts KW.whereText in node. stacked puts the namespace and the
     object on two lines, so a long name wraps on its own line. */
  KW.where = function (item, node, stacked) {
    node.classList.toggle("cluster", !item.namespace);
    var o = KW.obj(item.workloadKind || item.kind, item.namespace,
      item.workload);
    if (stacked && o) {
      node.textContent = "";
      node.appendChild(KW.el("span", "w-ns", item.namespace || "whole cluster"));
      node.appendChild(KW.el("span", "w-wl", o));
      return;
    }
    node.textContent = KW.whereText(item);
  };

  /* forLong: "for 3 h", how long an issue has been going on; "now"
     under a minute. */
  KW.forLong = function (fromIso) {
    var ms = KW.now() - Date.parse(fromIso);
    if (!(ms >= 60000)) return "now";
    return "for " + KW.lasted(fromIso, new Date(KW.now()).toISOString());
  };

  KW.failedAgain = function (n) {
    return "Failed again " + KW.plural(n, "time", "times");
  };

})();
