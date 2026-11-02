/* kwatch dashboard: search shortcuts. The search box takes plain words
   and a few shortcuts: ns:staging, kind:deployment, is:urgent (or
   attention, low, quiet), is:new, is:changed, is:acked, state:flapping,
   age:>1d, age:<2h, "a quoted phrase" and -word to leave matches out.
   parse turns the text into plain data; listquery.js sends what the API
   can filter, and keep applies the rest (is:new, is:changed: they live
   in this browser) to the page loaded. A
   shortcut it doesn't know is searched as plain text. No DOM here. */
(function () {
  "use strict";
  var KW = window.KW;

  var LEVELS = { urgent: "page", attention: "notify", low: "digest",
    quiet: "silent", page: "page", notify: "notify", digest: "digest",
    silent: "silent" };
  var MARKS = ["new", "changed", "acked", "stopped"];
  /* The most words (plain and -excluded together) the API takes. */
  var MAX_WORDS = 10;
  var STATES = ["open", "settling", "flapping", "recovering", "resolved"];
  var UNITS = { m: 60000, h: 3600000, d: 86400000, w: 604800000 };
  var NAMESPACE = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;

  /* tokens splits text on spaces; a "quoted phrase" is one token and a
     leading - negates a token. */
  function tokens(text) {
    var out = [], re = /(-?)"([^"]*)"?|(\S+)/g, m;
    while ((m = re.exec(text || "")) !== null) {
      if (m[3] === undefined) {
        if (m[2].trim()) out.push({ neg: !!m[1], text: m[2].trim(), quoted: true });
      } else if (m[3].length > 1 && m[3].charAt(0) === "-") {
        out.push({ neg: true, text: m[3].slice(1) });
      } else if (m[3] !== "-") {
        out.push({ neg: false, text: m[3] });
      }
    }
    return out;
  }

  function blank() {
    return { terms: [], exclude: [], namespace: [], kind: [], severity: [],
      state: [], is: {}, age: {} };
  }

  function add(list, v) {
    if (list.indexOf(v) < 0) list.push(v);
    return true;
  }

  /* shortcut applies one "prefix:value" token to s; false when it is
     not a shortcut, so the token is searched as text. */
  function shortcut(s, prefix, value, neg) {
    var v = value.toLowerCase();
    if (prefix === "is" && MARKS.indexOf(v) >= 0) {
      s.is[v] = !neg;
      return true;
    }
    if (neg) return false;
    if (prefix === "ns" || prefix === "namespace") {
      return v.length <= 63 && NAMESPACE.test(v) && add(s.namespace, v);
    }
    // The same kinds as ?kind=: "deployment", "ingress.networking.k8s.io".
    if (prefix === "kind") return /^[a-z][a-z0-9.]{0,62}$/.test(v) && add(s.kind, v);
    if (prefix === "is" && LEVELS[v]) return add(s.severity, LEVELS[v]);
    if (prefix === "state") return STATES.indexOf(v) >= 0 && add(s.state, v);
    var age = prefix === "age" && /^([<>])(\d{1,4})([mhdw])$/.exec(v);
    // age:>0d means nothing, and the API refuses it: ignore it.
    if (age && Number(age[2]) === 0) return true;
    if (age) {
      s.age[age[1] === ">" ? "over" : "under"] = Number(age[2]) * UNITS[age[3]];
      return true;
    }
    return false;
  }

  /* parse reads the search text into
     { terms, exclude, namespace, kind, severity, state, is, age }:
     terms must all be found, exclude must not be; is holds
     new / changed / acked / stopped as true or false; age.over and age.under
     are how long ago, in ms, the issue started. */
  function parse(text) {
    var s = blank();
    tokens(text).forEach(function (t) {
      var m = !t.quoted && /^([a-z]+):(.+)$/i.exec(t.text);
      if (m && shortcut(s, m[1].toLowerCase(), m[2], t.neg)) return;
      (t.neg ? s.exclude : s.terms).push(t.text);
    });
    // Words past the limit are left out, so the API never refuses it.
    s.terms = s.terms.slice(0, MAX_WORDS);
    s.exclude = s.exclude.slice(0, MAX_WORDS - s.terms.length);
    return s;
  }

  /* merge combines a picker's values with the shortcut's: both narrow
     the list, so the API gets the values in both. When none are in
     both, the picker's are sent and keep removes the rest. */
  function merge(picked, typed) {
    if (!picked.length) return typed.slice();
    if (!typed.length) return picked.slice();
    var both = picked.filter(function (v) { return typed.indexOf(v) >= 0; });
    return both.length ? both : picked.slice();
  }

  function seconds(ms) { return Math.round(ms / 1000) + "s"; }

  /* toAPI adds what the API filters to p. f is the view (the pickers),
     so a shortcut never widens what a picker narrowed. */
  function toAPI(p, s, f) {
    s.terms.forEach(function (t) { p.append("q", t); });
    s.exclude.forEach(function (t) { p.append("exclude", t); });
    merge(f.kind, s.kind).forEach(function (k) { p.append("kind", k); });
    merge(f.namespace, s.namespace).forEach(function (n) { p.append("namespace", n); });
    merge(f.severity, s.severity).forEach(function (v) { p.append("severity", v); });
    merge(f.state, s.state).forEach(function (v) { p.append("state", v); });
    var acked = s.is.acked === undefined ? [] : [s.is.acked ? "true" : "false"];
    merge(f.acked, acked).forEach(function (v) { p.append("acked", v); });
    var stopped = s.is.stopped === undefined ? [] : [s.is.stopped ? "true" : "false"];
    merge(f.stopped, stopped).forEach(function (v) { p.append("stopped", v); });
    if (!f.age && s.age.over) p.set("startedBefore", seconds(s.age.over));
    if (!f.age && s.age.under) p.set("startedAfter", seconds(s.age.under));
    return p;
  }

  function lacks(list, v) { return list.length > 0 && list.indexOf(v) < 0; }

  /* keep reports whether an issue the API returned passes every
     shortcut. marks are the New / Changed marks of visits.js. */
  function keep(item, s, marks, nowMs) {
    var mark = (marks || {})[item.id];
    var started = nowMs - (Date.parse(item.firstSeen) || nowMs);
    var is = s.is;
    return !(lacks(s.namespace, item.namespace || "") ||
      lacks(s.kind, (item.workloadKind || "").toLowerCase()) ||
      lacks(s.severity, item.severity) || lacks(s.state, item.state) ||
      (is.acked !== undefined && is.acked !== !!item.ackedBy) ||
      (is.stopped !== undefined && is.stopped !== !!item.stoppedTracking) ||
      (is.new !== undefined && is.new !== !!(mark && mark.what === "new")) ||
      (is.changed !== undefined &&
        is.changed !== !!(mark && mark.what !== "new")) ||
      (s.age.over && started <= s.age.over) ||
      (s.age.under && started >= s.age.under));
  }

  KW.searchQuery = { parse: parse, toAPI: toAPI, keep: keep };
})();
