/* kwatch dashboard: Issue detail, in the order of a kwatch alert. */
(function () {
  "use strict";
  var KW = window.KW;
  var EVERY = 30000;
  var MAX_TIMELINE = 100;
  /* The newest events show first; older ones wait behind a button. */
  var TIMELINE_SHOWN = 8;

  var BANNERS = {
    flapping: ["n-degraded", "Keeps coming back",
      "This keeps failing and recovering. kwatch stays quiet about it unless it changes."],
    recovering: ["n-resolved", "Looks better",
      "It looks fixed. kwatch is waiting a little to be sure before it calls it resolved."]
  };

  function upperFirst(s) { return s ? s.charAt(0).toUpperCase() + s.slice(1) : s; }

  function after(text, prefix) {
    return text.indexOf(prefix) === 0 ? text.slice(prefix.length) : text;
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
        return [upperFirst(t), "k-warn"];
      case "cause_revised":
        return [causeWords(after(after(t, "cause revised"), ": ")), ""];
      case "same_outage":
        return ["Part of the same outage as " + after(t, "same outage as ").replace(/, already paged$/, "") +
          ", which kwatch already alerted about", ""];
      case "flapping":
        return ["Keeps failing and recovering: " + after(t, "flapping: "), "k-warn"];
      case "paged_again":
        return ["Still failing, so kwatch alerted again", "k-warn"];
      case "paged_together":
        return ["Sent with the urgent alert for the same moment", ""];
      case "resolved":
        if (ev.reason === "stopped_tracking") {
          return ["kwatch stopped tracking it: " + after(t, "resolved: stopped tracking after "), "k-done"];
        }
        return ["Resolved: " + (RESOLVED[ev.reason] || upperFirst(after(t, "resolved: "))), "k-done"];
      case "acknowledged":
      case "ack_removed":
        return null;
    }
    return [upperFirst(t), ""];
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
    var tail = e.slice(e.indexOf("/") + 1);
    if (text.indexOf(e) >= 0 || text.indexOf(tail) >= 0) return "";
    if (inc.namespace && inc.workload &&
        tail === inc.namespace + "/" + inc.workload.split("/").pop()) return "";
    return e;
  }

  /* timeline: newest first, grouped by day. Back-to-back events that say
     the same thing collapse into one row with a count; a different event
     in between still splits them, so the order stays true. The newest
     TIMELINE_SHOWN rows show; "Show N older events" opens the rest and
     stays open across refreshes (ol.dataset.open). */
  function timeline(ol, more, inc) {
    ol.textContent = "";
    var events = [{ at: inc.firstSeen, kind: "opened", text: "kwatch noticed this" }];
    var raw = inc.timeline || [];
    var cut = raw.length >= MAX_TIMELINE;
    if (cut) events = [];
    events = events.concat(raw).map(function (ev, i) {
      var w = ev.kind === "opened" ? [ev.text, "k-start"] : words(ev);
      return w && KW.valid(ev.at) ? { ev: ev, w: w, i: i } : null;
    }).filter(Boolean);
    // Newest first; same-moment events keep their recorded order reversed.
    events.sort(function (a, b) {
      return (Date.parse(b.ev.at) - Date.parse(a.ev.at)) || (b.i - a.i);
    });
    var rows = [];
    events.forEach(function (e) {
      var key = e.ev.kind + "|" + e.w[0] + "|" + (e.ev.entity || "");
      var last = rows[rows.length - 1];
      // kwatch itself folds old repeats into one event ("repeats"); count
      // them with the ones shown here.
      var folded = e.ev.repeats || 0;
      if (last && last.key === key) {
        last.all.push(e.ev.at);
        last.folded += folded;
      } else {
        rows.push({ key: key, e: e, all: [e.ev.at], folded: folded });
      }
    });
    var open = ol.dataset.open === "1";
    var shown = open ? rows : rows.slice(0, TIMELINE_SHOWN);
    var lastDay = "";
    shown.forEach(function (row, n) {
      var ev = row.e.ev, w = row.e.w;
      var d = new Date(ev.at);
      var day = dayLabel(d);
      if (day !== lastDay) {
        var dl = KW.el("li", "day", day);
        dl.setAttribute("aria-hidden", "true");
        ol.appendChild(dl);
        lastDay = day;
      }
      var li = KW.el("li", "ev" + (w[1] ? " " + w[1] : "") + (n === 0 ? " latest" : ""));
      var t = KW.el("span", "t");
      var te = KW.el("time", "", KW.hhmm(d));
      te.setAttribute("datetime", ev.at);
      te.title = KW.full(ev.at);
      t.appendChild(te);
      li.appendChild(t);
      li.appendChild(KW.el("span", "node"));
      var tx = KW.el("span", "tx");
      tx.appendChild(KW.el("span", "tx-text", w[0]));
      var count = row.all.length + row.folded;
      if (count > 1) tx.appendChild(KW.el("span", "tl-count", "× " + count));
      if (n === 0) tx.appendChild(KW.el("span", "tl-latest", "Latest"));
      // "2 h ago" helps today; on earlier days the day and time say it.
      var meta = KW.el("small");
      function sep() { if (meta.firstChild) meta.appendChild(document.createTextNode(" · ")); }
      if (row.all.length > 1) {
        meta.appendChild(document.createTextNode("since " + KW.stamp(row.all[row.all.length - 1])));
      } else if (KW.isToday(d)) {
        meta.appendChild(KW.timeEl(ev.at));
      }
      var entity = entityShown(ev, w[0], inc);
      if (entity) { sep(); meta.appendChild(KW.el("span", "mono", entity)); }
      if (meta.firstChild) tx.appendChild(meta);
      if (count > 1) {
        var det = KW.el("details", "tl-times");
        det.appendChild(KW.el("summary", "", "Show all times"));
        var times = KW.el("p", "mono");
        times.textContent = row.all.map(function (at) { return KW.stamp(at); }).join(" · ") +
          (row.folded ? " · and " + row.folded + " earlier" : "");
        det.appendChild(times);
        tx.appendChild(det);
      }
      li.appendChild(tx);
      ol.appendChild(li);
    });
    var hidden = rows.length - shown.length;
    if (cut && (open || hidden === 0)) {
      ol.appendChild(KW.el("li", "earlier", "Older events are not shown."));
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

  function logClass(line) {
    var l = line.toLowerCase();
    if (/(fatal|panic|error|exception|failed|oomkilled)/.test(l)) return "e";
    if (/(warn|timeout|retry)/.test(l)) return "w";
    return "r";
  }

  function steps(box, inc) {
    box.textContent = "";
    (inc.fixSteps || []).forEach(function (s) {
      var div = KW.el("div", "step");
      if (s.text) div.appendChild(KW.el("p", "", s.text));
      if (s.command) div.appendChild(KW.cmd(s.command));
      box.appendChild(div);
    });
  }

  function runbook(p, link) {
    p.textContent = "";
    p.hidden = !link;
    if (!link) return;
    var scheme = link.slice(0, link.indexOf(":") + 1).toLowerCase();
    var safe = scheme === "https:" || scheme === "http:";
    if (!safe) {
      p.textContent = "Your runbook for this: " + link;
      return;
    }
    var a = KW.el("a", "ulink", "Your runbook for this");
    a.href = link;
    a.target = "_blank";
    a.rel = "noopener noreferrer";
    p.appendChild(a);
  }

  function facts(dl, inc) {
    dl.textContent = "";
    function add(label, value, cls) {
      dl.appendChild(KW.el("dt", "", label));
      var dd = KW.el("dd", cls || "");
      if (typeof value === "string") dd.textContent = value;
      else dd.appendChild(value);
      dl.appendChild(dd);
    }
    var where = KW.el("span");
    KW.where(inc, where);
    add("Where", where, inc.namespace ? "mono" : "");
    // Today: "14:02, 3 h ago". Earlier days already read as a date.
    var started = KW.el("span");
    if (KW.isToday(new Date(inc.firstSeen))) {
      started.textContent = KW.hhmm(new Date(inc.firstSeen)) + ", ";
    }
    started.appendChild(KW.timeEl(inc.firstSeen));
    add("Started", started);
    add("Last seen", KW.timeEl(inc.lastSeen));
    if (inc.repeatCount > 0) add("Failed again", KW.plural(inc.repeatCount, "time", "times"));
    if (inc.state === "resolved" && KW.valid(inc.resolvedAt)) {
      add("Resolved", KW.stamp(inc.resolvedAt) + " (lasted " +
        KW.lasted(inc.firstSeen, inc.resolvedAt) + ")");
    }
    add("Issue ID", inc.id, "mono");
  }

  /* pager: Previous / Next through the list this issue was opened from,
     in its on-screen order. Hidden when there is no such list. */
  function pager(root, inc, kind) {
    var ids = KW.listIds[kind] || [];
    var i = ids.indexOf(inc.id);
    var box = KW.$("[data-pager]", root);
    box.hidden = i < 0 || ids.length < 2;
    if (box.hidden) return;
    KW.$("[data-pos]", root).textContent = (i + 1) + " of " + ids.length;
    [["[data-prev]", ids[i - 1]], ["[data-next]", ids[i + 1]]].forEach(function (p) {
      var a = KW.$(p[0], root);
      if (p[1]) {
        a.setAttribute("href", "/issues/" + encodeURIComponent(p[1]));
        a.removeAttribute("aria-disabled");
      } else {
        a.removeAttribute("href");
        a.setAttribute("aria-disabled", "true");
      }
    });
  }

  function fill(root, inc) {
    var resolved = inc.state === "resolved";
    KW.title(inc.title);
    KW.setNav(resolved ? "resolved" : "active");
    var back = KW.$("[data-back]", root);
    back.setAttribute("href", resolved ? KW.lastList.resolved : KW.lastList.active);
    KW.$("span", back).textContent = resolved ? "Resolved" : "Active issues";
    pager(root, inc, resolved ? "resolved" : "active");

    var banner = KW.$("[data-banner]", root);
    var b = BANNERS[inc.state];
    if (resolved && KW.valid(inc.resolvedAt)) {
      b = ["n-success", "Resolved", "Resolved " + KW.stamp(inc.resolvedAt) +
        ". kwatch keeps resolved issues for 7 days."];
    }
    banner.hidden = !b;
    if (b) {
      banner.className = "notice d-banner " + b[0];
      KW.$("b", banner).textContent = b[1];
      KW.$("p", banner).textContent = b[2];
    }
    var chips = KW.$("[data-chips]", root);
    chips.textContent = "";
    if (resolved) {
      chips.appendChild(KW.chip("c-resolved", "Resolved"));
      chips.appendChild(KW.severityChip(inc.severity, "Was "));
    } else {
      chips.appendChild(KW.severityChip(inc.severity));
      // "Happening now" is the normal state; only other states are news.
      if (inc.state !== "open") chips.appendChild(KW.stateChip(inc.state));
    }
    KW.$("[data-title]", root).textContent = inc.title;
    // A link to this issue, to paste into chat or a ticket.
    var copyLink = KW.$("[data-copy-link]", root);
    copyLink.onclick = function () {
      KW.copy(copyLink, location.origin + "/issues/" + encodeURIComponent(inc.id), "Copy link");
    };

    var why = KW.$("[data-why]", root);
    var cause = inc.rootCause || (inc.state === "settling" ?
      "kwatch hasn't found a single cause yet. It is still collecting what failed around it." : "");
    why.hidden = !cause;
    KW.$("p", why).textContent = cause;

    var fix = KW.$("[data-fix]", root);
    steps(KW.$("[data-steps]", root), inc);
    runbook(KW.$("[data-runbook]", root), inc.docsLink);
    fix.hidden = !(inc.fixSteps && inc.fixSteps.length) && !inc.docsLink;

    var logs = KW.$("[data-logs]", root);
    var keep = logs.scrollTop, keepX = logs.scrollLeft;
    var lines = inc.capturedLogLines || [];
    logs.textContent = "";
    lines.forEach(function (l) { logs.appendChild(KW.el("div", logClass(l), l)); });
    logs.hidden = lines.length === 0;
    logs.scrollTop = keep;
    logs.scrollLeft = keepX;
    // Nothing captured: leave the section out rather than say so.
    KW.$("[data-logs-sec]", root).hidden = lines.length === 0;
    KW.$("[data-logcap]", root).textContent = "Captured when this started. Secrets are removed.";
    var copyLogs = KW.$("[data-copy-logs]", root);
    copyLogs.hidden = lines.length === 0;
    copyLogs.onclick = function () { KW.copy(copyLogs, lines.join("\n"), "Copy all"); };
    // Long lines scroll sideways by default; wrapping reads easier.
    var wrap = KW.$("[data-wrap-logs]", root);
    wrap.hidden = lines.length === 0;
    wrap.onclick = function () {
      var on = logs.classList.toggle("wrap");
      wrap.setAttribute("aria-pressed", on ? "true" : "false");
    };

    timeline(KW.$("[data-timeline]", root), KW.$("[data-tl-more]", root), inc);
    facts(KW.$("[data-facts]", root), inc);
    KW.evidence(root, inc);
  }

  function notFound(main) {
    KW.title("Issue not found");
    main.textContent = "";
    main.appendChild(KW.el("h1", "", "We can't find that issue"));
    var p = KW.el("p", "lead-404", "kwatch only keeps resolved issues for 7 days, and the ID may be mistyped.");
    main.appendChild(p);
    var links = KW.el("p", "links-404");
    [["/", "Active issues"], ["/resolved", "Resolved"]].forEach(function (l, i) {
      if (i) links.appendChild(document.createTextNode(" · "));
      var a = KW.el("a", "ulink", l[1]);
      a.href = l[0];
      a.dataset.link = "";
      links.appendChild(a);
    });
    main.appendChild(links);
  }

  KW.detailView = function (main, r) {
    var root = null, attempt = 0, lastBody = "", stopped = false;
    var scrollBack = KW.takeScroll();
    var alertBox = KW.el("div");
    main.appendChild(alertBox);
    var slow = window.setTimeout(function () {
      if (root) return;
      var list = KW.el("ul", "list");
      main.appendChild(list);
      for (var i = 0; i < 3; i++) {
        var li = KW.el("li", "skel");
        li.appendChild(KW.el("span"));
        li.appendChild(KW.el("span"));
        list.appendChild(li);
      }
      list.appendChild(KW.el("li", "sr-only", "Loading"));
    }, 300);

    function load() {
      KW.refreshBadge();
      return KW.api("/api/incidents/" + encodeURIComponent(r.id)).then(function (res) {
        // An answer that arrives after the page was left changes nothing.
        if (stopped) return 0;
        window.clearTimeout(slow);
        if (KW.authFailed(res)) return 0;
        if (res.status === 404) { notFound(main); poll.stop(); return 0; }
        alertBox.textContent = "";
        if (res.status !== 200 || !res.data) {
          var k = KW.kindOf(res.status);
          if (!root) {
            main.textContent = "";
            main.appendChild(alertBox);
          }
          alertBox.appendChild(KW.panel(k, res.status, function () { poll.now(); }));
          attempt += 1;
          return KW.retryDelay(k, attempt);
        }
        attempt = 0;
        var body = JSON.stringify(res.data);
        if (body === lastBody) return 0;
        lastBody = body;
        if (!root) {
          main.textContent = "";
          main.appendChild(alertBox);
          var frag = KW.clone("tpl-detail");
          root = KW.el("div");
          root.appendChild(frag);
          main.appendChild(root);
        }
        fill(root, res.data);
        if (scrollBack) {
          window.scrollTo(0, scrollBack);
          scrollBack = 0;
        }
        return 0;
      });
    }
    var poll = KW.poller(load, EVERY);
    return { stop: function () { stopped = true; poll.stop(); window.clearTimeout(slow); } };
  };
})();
