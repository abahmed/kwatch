/* kwatch dashboard: Issue detail, in the order of a kwatch alert. */
(function () {
  "use strict";
  var KW = window.KW;
  var EVERY = 30000;
  var MAX_TIMELINE = 100;

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
        return ["kwatch found a different cause: " + after(after(t, "cause revised"), ": "), ""];
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

  function dayLabel(d) {
    if (KW.isToday(d)) return "Today, " + KW.dayMonth(d);
    if (KW.isYesterday(d)) return "Yesterday, " + KW.dayMonth(d);
    return KW.dayMonth(d);
  }

  function timeline(ol, inc) {
    ol.textContent = "";
    var events = [{ at: inc.firstSeen, kind: "opened", text: "kwatch noticed this" }];
    var raw = inc.timeline || [];
    if (raw.length >= MAX_TIMELINE) {
      ol.appendChild(KW.el("li", "earlier", "Earlier events are not shown."));
      events = [];
    }
    events = events.concat(raw);
    var lastDay = "";
    events.forEach(function (ev) {
      var w = ev.kind === "opened" ? [ev.text, ""] : words(ev);
      if (!w || !KW.valid(ev.at)) return;
      var d = new Date(ev.at);
      var day = dayLabel(d);
      if (day !== lastDay) {
        var dl = KW.el("li", "day", day);
        dl.setAttribute("aria-hidden", "true");
        ol.appendChild(dl);
        lastDay = day;
      }
      var li = KW.el("li", "ev" + (w[1] ? " " + w[1] : ""));
      var t = KW.el("span", "t");
      var te = KW.el("time", "", KW.hhmm(d));
      te.setAttribute("datetime", ev.at);
      te.title = KW.full(ev.at);
      t.appendChild(te);
      li.appendChild(t);
      li.appendChild(KW.el("span", "node"));
      var tx = KW.el("span", "tx", w[0]);
      if (ev.entity) tx.appendChild(KW.el("small", "", ev.entity));
      li.appendChild(tx);
      ol.appendChild(li);
    });
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
    var started = KW.el("span", "", KW.hhmm(new Date(inc.firstSeen)) + ", ");
    started.appendChild(KW.timeEl(inc.firstSeen));
    add("Started", started);
    add("Last seen", KW.timeEl(inc.lastSeen));
    add("Failed again", inc.repeatCount > 0 ? KW.plural(inc.repeatCount, "time", "times") : "Not yet");
    if (inc.state === "resolved" && KW.valid(inc.resolvedAt)) {
      add("Resolved", KW.stamp(inc.resolvedAt) + " (lasted " +
        KW.lasted(inc.firstSeen, inc.resolvedAt) + ")");
    }
    add("Issue ID", inc.id, "mono");
  }

  function fill(root, inc) {
    var resolved = inc.state === "resolved";
    KW.title(inc.title);
    KW.setNav(resolved ? "resolved" : "active");
    var back = KW.$("[data-back]", root);
    back.setAttribute("href", resolved ? KW.lastList.resolved : KW.lastList.active);
    KW.$("span", back).textContent = resolved ? "Resolved" : "Active issues";

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
      chips.appendChild(KW.stateChip(inc.state));
    }
    KW.$("[data-title]", root).textContent = inc.title;

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
    KW.$("[data-logcap]", root).textContent = lines.length ?
      "Last lines kwatch captured when this started. Known secret patterns are removed." :
      "No log lines were captured for this issue.";
    var copyLogs = KW.$("[data-copy-logs]", root);
    copyLogs.hidden = lines.length === 0;
    copyLogs.onclick = function () { KW.copy(copyLogs, lines.join("\n"), "Copy all"); };

    timeline(KW.$("[data-timeline]", root), inc);
    facts(KW.$("[data-facts]", root), inc);
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
    var root = null, attempt = 0, lastBody = "";
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
        return 0;
      });
    }
    var poll = KW.poller(load, EVERY);
    return { stop: function () { poll.stop(); window.clearTimeout(slow); } };
  };
})();
