/* kwatch dashboard: an issue's timeline ("What happened, step by
   step"), newest first, in plain words. */
(function () {
  "use strict";
  var KW = window.KW;
  var MAX_TIMELINE = 100;
  /* The newest events show first; older ones wait behind a button. */
  var TIMELINE_SHOWN = 8;

  /* KW.hideMarks puts text into node as text nodes, with each place
     kwatch hid a secret ("[redacted]") as a small "hidden" tag. */
  KW.hideMarks = function (node, text) {
    var parts = String(text).split(/(\[redacted(?: private key)?\])/);
    parts.forEach(function (part, i) {
      if (i % 2 === 0) {
        if (part) node.appendChild(document.createTextNode(part));
        return;
      }
      var tag = node.appendChild(KW.el("span", "hid-tag", "hidden"));
      tag.title = "kwatch hid a secret here";
    });
  };

  /* upperFirst starts a sentence with a capital, unless it starts with
     an object's name ("payments-api-7d9f… was OOMKilled"): a name is
     never changed. */
  function upperFirst(s, ev) {
    if (!s) return s;
    var first = s.split(" ")[0], e = (ev && ev.entity) || "";
    var name = e.slice(e.lastIndexOf("/") + 1);
    if (/[-\/.:_0-9]/.test(first) || (name && s.indexOf(name) === 0)) return s;
    return s.charAt(0).toUpperCase() + s.slice(1);
  }

  function after(text, prefix) {
    return text.indexOf(prefix) === 0 ? text.slice(prefix.length) : text;
  }

  /* lastTime: an older kwatch named the earlier issue by its ID ("last
     time inc-20261029-5c97-13940"); the ID's date says when, in words.
     The ID's date is a UTC day, so it is counted against today's UTC
     day, and shown at noon UTC so either clock names the same day. */
  function lastTime(t) {
    return t.replace(/last time inc-(\d{4})(\d\d)(\d\d)-[0-9a-z]+-\d+/g, function (all, y, m, d) {
      var day = Date.UTC(+y, +m - 1, +d), now = new Date(KW.now());
      var today = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate());
      var n = Math.round((today - day) / 86400000);
      if (isNaN(n) || n < 0) return "last time on " + KW.dayMonth(new Date(day + 43200000));
      return "last time " + (n === 0 ? "today" : n === 1 ? "yesterday" : n + " days ago");
    });
  }

  var RESOLVED = {
    recovered: "It stayed healthy.",
    recovered_unheard: "It recovered before kwatch had to alert anyone.",
    superseded: "Folded into another issue with the same cause."
  };

  /* words: what a timeline event says, by its kind. Unknown text is
     shown unchanged, never dropped. Acknowledgements are not shown. */
  function words(ev) {
    var t = ev.text || "";
    switch (ev.kind) {
      case "repeated":
        if (t.indexOf("failing again: ") === 0) return ["Failed again: " + after(t, "failing again: "), "k-warn"];
        return [upperFirst(lastTime(t), ev), "k-warn"];
      case "cause_revised":
        return [causeWords(after(after(t, "cause revised"), ": ")), ""];
      case "same_outage":
        return ["Part of the same outage as " + after(t, "same outage as ").replace(/, already paged$/, "") +
          ", which kwatch already alerted about", ""];
      case "flapping":
        return ["Keeps failing and recovering: " + after(t, "flapping: "), "k-warn"];
      case "paged_again":
        return ["Still happening, so kwatch alerted again", "k-warn"];
      case "paged_together":
        return ["Sent with the urgent alert for the same moment", ""];
      case "resolved":
        if (ev.reason === "stopped_tracking") {
          return ["kwatch stopped tracking it: " + after(t, "resolved: stopped tracking after "), "k-done"];
        }
        return ["Resolved: " + (RESOLVED[ev.reason] || upperFirst(after(t, "resolved: "), ev)), "k-done"];
      case "acknowledged":
      case "ack_removed":
        return null;
      case "transition":
        return [t, ev.cls || ""];
    }
    return [upperFirst(t, ev), ""];
  }

  /* causeWords turns kwatch's cause notes into plain sentences. Unknown
     notes keep their text. */
  function causeWords(rest) {
    if (rest === "only digest findings are left") {
      return "Lowered to Low: only minor problems are left";
    }
    if (rest.indexOf("now explained by ") === 0) {
      return "Cause updated: " + after(rest, "now explained by ");
    }
    if (rest.indexOf("its failures moved to ") === 0) {
      return "Moved: now tracked as part of " + after(rest, "its failures moved to ");
    }
    var m = / is now explained by /.exec(rest);
    if (m) {
      return "Cause updated: " + rest.slice(0, m.index) + " is now caused by " +
        rest.slice(m.index + m[0].length);
    }
    return "kwatch found a different cause: " + rest;
  }

  function dayLabel(d) {
    if (KW.isToday(d)) return "Today, " + KW.dayMonth(d);
    if (KW.isYesterday(d)) return "Yesterday, " + KW.dayMonth(d);
    return KW.dayMonth(d);
  }

  /* entityShown: the object an event names, unless the event text
     already names it or it is the issue's own workload. */
  function entityShown(ev, text, inc) {
    var e = ev.entity || "";
    if (!e) return "";
    var tail = e.slice(e.indexOf("/") + 1), name = e.slice(e.lastIndexOf("/") + 1);
    if (text.indexOf(e) >= 0 || text.indexOf(tail) >= 0 || text.indexOf(name) >= 0) return "";
    if (inc.namespace && inc.workload &&
        tail === inc.namespace + "/" + inc.workload.split("/").pop()) return "";
    return e;
  }

  /* TAIL: the object an event names at its end, "(container ns/pod/c)":
     it is said once above the list, not in every line. */
  var TAIL = /\s*\((?:container|pod|node|deployment|replicaset|statefulset|daemonset)[^)]*\)\s*$/;
  function clean(t) { return t.replace(TAIL, ""); }
  var GROUPED = 3;

  /* podOf: an entity key as the pod it belongs to, so a pod and its
     container count as one object: "container/ns/pod/c" → "pod/ns/pod". */
  function podOf(key) {
    var p = key.split("/");
    return p[0] === "container" && p.length >= 4 ? "pod/" + p[1] + "/" + p[2] : key;
  }

  /* signature: what makes two events the same thing happening again:
     their words without the object, the numbers and the detail after the
     first colon; a failure and its "recovered:" share one. */
  function signature(ev, text) {
    var t = clean(text).replace(/^recovered:\s*/i, "").replace(/\d+/g, "#"), at = t.indexOf(": ");
    return ev.kind + "|" + (at >= 12 ? t.slice(0, at) : t);
  }

  function groupTitle(text) {
    var t = clean(text).replace(/^recovered:\s*/i, ""), at = t.indexOf(": ");
    return at >= 12 ? t.slice(0, at) : t;
  }

  /* groupText: "Container keeps failing its readiness probe · 28 times
     between 10:35 and 11:20, recovered 27 times". */
  function groupText(events) {
    var fails = events.filter(function (e) { return !/^recovered:/i.test(e.w[0]); });
    var at = events.map(function (e) { return Date.parse(e.ev.at); });
    var first = new Date(Math.min.apply(null, at)).toISOString(), last = new Date(Math.max.apply(null, at)).toISOString();
    var back = events.length - fails.length;
    var title = groupTitle((fails[0] || events[0]).w[0]);
    var n = fails.length || back;
    var text = title + " · " + KW.plural(n, "time", "times") + " between " + KW.when(first) + " and " + KW.when(last);
    if (fails.length && back) text += ", recovered " + KW.plural(back, "time", "times");
    return text;
  }

  /* timeline: newest first, grouped by day. Back-to-back events that say
     the same thing collapse into one row with a count; a different event
     in between still splits them, so the order stays true. Each event
     is its own line; lines in the same minute as the one above share
     its time label (shown once). The newest
     TIMELINE_SHOWN rows show; "Show N older events" opens the rest and
     stays open across refreshes (ol.dataset.open). */
  function timeline(ol, more, inc) {
    ol.textContent = "";
    var events = [{ at: inc.firstSeen, kind: "opened", text: "kwatch noticed this" }];
    var raw = (inc.timeline || []).concat(KW.transitionEvents(inc));
    var dropped = KW.cutCount(inc, "timeline");
    var cut = dropped > 0 || (inc.timeline || []).length >= MAX_TIMELINE;
    if (cut) events = [];
    events = events.concat(raw).map(function (ev, i) {
      var w = ev.kind === "opened" ? [ev.text, "k-start"] : words(ev);
      return w && KW.valid(ev.at) ? { ev: ev, w: w, i: i } : null;
    }).filter(Boolean);
    // Newest first; same-moment events keep their recorded order reversed.
    events.sort(function (a, b) {
      return (Date.parse(b.ev.at) - Date.parse(a.ev.at)) || (b.i - a.i);
    });
    var sigs = {};
    events.forEach(function (e) {
      if (e.ev.kind === "opened" || e.ev.kind === "transition") return;
      e.sig = signature(e.ev, e.w[0]);
      (sigs[e.sig] = sigs[e.sig] || []).push(e);
    });
    var rows = [], done = {};
    events.forEach(function (e) {
      var many = e.sig && sigs[e.sig].length >= GROUPED;
      if (many) {
        if (done[e.sig]) return;
        done[e.sig] = true;
        var all = sigs[e.sig];
        rows.push({ key: e.sig, e: { ev: e.ev, w: [groupText(all), e.w[1]], i: e.i }, group: true,
          all: all.map(function (x) { return x.ev.at; }), folded: 0 });
        return;
      }
      var key = e.ev.kind + "|" + e.w[0] + "|" + (e.ev.entity || "");
      var last = rows[rows.length - 1];
      // kwatch itself folds old repeats into one event ("repeats"); count
      // them with the ones shown here.
      var folded = e.ev.repeats || 0;
      if (last && !last.group && last.key === key) {
        last.all.push(e.ev.at);
        last.folded += folded;
      } else {
        rows.push({ key: key, e: e, all: [e.ev.at], folded: folded });
      }
    });
    // One object in every event: say so once, above the list.
    var objects = {};
    events.forEach(function (e) { if (e.ev.entity) objects[podOf(e.ev.entity)] = true; });
    var names = Object.keys(objects), single = names.length === 1 ? names[0] : "";
    ol.dataset.rows = String(rows.length);
    var open = ol.dataset.open === "1";
    var shown = open ? rows : rows.slice(0, TIMELINE_SHOWN);
    var lastDay = "", lastHM = "";
    if (single) {
      var about = ol.appendChild(KW.el("li", "earlier tl-about", "About "));
      about.appendChild(KW.wrapName(single));
    }
    shown.forEach(function (row, n) {
      var ev = row.e.ev, w = row.e.w;
      var d = new Date(ev.at);
      var day = dayLabel(d);
      if (day !== lastDay) {
        var dl = KW.el("li", "day", day);
        dl.setAttribute("aria-hidden", "true");
        ol.appendChild(dl);
        lastDay = day;
        lastHM = "";
      }
      var hm = KW.hhmm(d), sameT = hm === lastHM;
      lastHM = hm;
      var fresh = row.all.some(function (at) { return KW.isNew(at); });
      var li = KW.el("li", "ev" + (w[1] ? " " + w[1] : "") + (n === 0 ? " latest" : "") +
        (fresh ? " is-new" : "") + (sameT ? " same-t" : ""));
      var t = KW.el("span", "t");
      var te = KW.el("time", "", hm);
      te.setAttribute("datetime", ev.at);
      te.title = KW.full(ev.at);
      t.appendChild(te);
      li.appendChild(t);
      li.appendChild(KW.el("span", "node"));
      var tx = KW.el("span", "tx");
      KW.hideMarks(tx.appendChild(KW.el("span", "tx-text")), single || row.group ? clean(w[0]) : w[0]);
      var count = row.all.length + row.folded;
      if (count > 1 && !row.group) {
        tx.appendChild(KW.el("span", "tl-count", KW.plural(count, "time", "times"))).title =
          "Happened " + KW.plural(count, "time", "times") + " in a row";
      }
      if (n === 0) tx.appendChild(KW.el("span", "tl-latest", "Latest"));
      if (fresh) tx.appendChild(KW.newMark());
      // "2 h ago" helps today; on earlier days the day and time say it.
      var meta = KW.el("small");
      function sep() { if (meta.firstChild) meta.appendChild(document.createTextNode(" · ")); }
      if (row.all.length > 1 && !row.group) {
        meta.appendChild(document.createTextNode("since " + KW.stamp(row.all[row.all.length - 1])));
      } else if (KW.isToday(d)) {
        meta.appendChild(KW.timeEl(ev.at));
      }
      var entity = row.group || single && (!ev.entity || podOf(ev.entity) === single) ? "" : entityShown(ev, w[0], inc);
      if (entity) { sep(); meta.appendChild(KW.el("span", "mono", entity)); }
      if (meta.firstChild) tx.appendChild(meta);
      if (count > 1) {
        var det = KW.el("details", "tl-times");
        det.appendChild(KW.el("summary", "", row.group ? "Show every time" : "Show all times"));
        var times = KW.el("p", "mono");
        // Today's stamps are just the time; a stamp never breaks inside.
        row.all.forEach(function (at, i) {
          if (i) times.appendChild(document.createTextNode(" · "));
          times.appendChild(KW.el("span", "tl-stamp", KW.when(at)));
        });
        if (row.folded) times.appendChild(document.createTextNode(" · and " + row.folded + " earlier"));
        det.appendChild(times);
        tx.appendChild(det);
      }
      li.appendChild(tx);
      ol.appendChild(li);
    });
    var tCut = KW.cutCount(inc, "transitions");
    if (tCut) ol.appendChild(KW.el("li", "earlier", KW.plural(tCut, "state change", "state changes") + " not shown."));
    var hidden = rows.length - shown.length;
    if (cut && (open || hidden === 0)) {
      ol.appendChild(KW.el("li", "earlier", dropped ?
        KW.plural(dropped, "older event", "older events") + " not shown." :
        "Older events are not shown."));
    }
    more.hidden = rows.length <= TIMELINE_SHOWN;
    more.setAttribute("aria-expanded", open ? "true" : "false");
    more.textContent = open ? "Show fewer" :
      "Show " + KW.plural(hidden, "older event", "older events");
    more.onclick = function () {
      ol.dataset.open = open ? "" : "1";
      timeline(ol, more, inc);
    };
  }

  KW.timeline = timeline;
})();
