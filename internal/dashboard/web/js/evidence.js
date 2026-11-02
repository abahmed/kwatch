/* kwatch dashboard: the evidence of an issue (Pro), under "Why kwatch
   thinks this", its Ingress-to-pods chain under "Affected objects", and
   "Logs": the lines kept when it started and the logs the checks read.
   Every string from the API is set with textContent; log text is never
   interpreted. */
(function () {
  "use strict";
  var KW = window.KW;

  var BADGE = {
    ok: ["Read", "b-read"], denied: ["Not allowed", "b-warn"],
    timeout: ["Timed out", "b-warn"], absent: ["Nothing to read", "b-read"],
    error: ["Couldn't read", "b-warn"]
  };

  var DENIED = {
    logs: "kwatch isn't allowed to read pod logs. The rest of the evidence is shown. " +
      "To allow it, set rbac.mode and check that pods/log is granted in your Role.",
    config: "kwatch isn't allowed to read Secret names here. Other checks are shown. " +
      "To allow it, set watch.secrets: true in your Helm values.",
    "control-plane": "kwatch isn't allowed to read the control plane's readiness endpoint. " +
      "To allow it, set rbac.readyz: true in your Helm values."
  };
  var DENIED_OTHER = "kwatch isn't allowed to read this. To allow it, set rbac.mode: full " +
    "in your Helm values, or grant get on it in your Role.";
  var OFF = "Log capture is turned off (investigation.logs: false). " +
    "Set investigation.logs: true to show logs here.";
  var TIMEOUT = "This check took longer than its time limit, so kwatch moved on. " +
    "The other checks are shown. kwatch looks again at the next update.";
  var ABSENT = {
    ingress: "No Ingress routes to this Service.",
    admission: "No admission webhook touches this object."
  };
  var FROM = { "Signature": "logs", "Spread": "logs", "Matches": "service",
    "Change": "rollout", "Before and after": "rollout" };

  /* newest: the latest readAt of list, or "". */
  function newest(list) {
    var iso = list.map(function (s) { return s.readAt; }).filter(Boolean).sort().pop();
    return iso && KW.valid(iso) ? iso : "";
  }

  /* readAt is the "Read at 13:44" footer of a section (the seconds are
     in its tooltip), or null. */
  function readAt(list) {
    var iso = newest(list);
    if (!iso) return null;
    var p = KW.el("p", "ev-cap", "Read at ");
    p.appendChild(KW.hm(iso));
    return p;
  }

  function badge(s) {
    if (s.status === "denied" && s.reason === "off") return ["Turned off", "b-warn"];
    return BADGE[s.status] || BADGE.absent;
  }

  function bodyText(s) {
    if (s.status === "denied") return s.reason === "off" ? OFF : (DENIED[s.check] || DENIED_OTHER);
    if (s.status === "timeout") return TIMEOUT;
    if (s.status === "error") return "kwatch couldn't read this right now. The other checks are shown. " +
      "kwatch looks again at the next update.";
    if (s.status !== "absent") return "";
    if (s.check === "logs") {
      return s.run === "previous" ? "No previous run. This container has not restarted." :
        "This container has written nothing yet.";
    }
    return ABSENT[s.check] || "Nothing to read here.";
  }

  /* summary: what the checks managed, in plain words: "kwatch checked
     3 things: it read 2 and it isn't allowed to read 1." */
  function summary(sections) {
    var checks = {}, n = 0, read = 0, denied = 0, slow = 0;
    sections.forEach(function (s) {
      if (checks[s.check]) return;
      checks[s.check] = true;
      n += 1;
      if (s.status === "ok" || s.status === "absent") read += 1;
      if (s.status === "denied") denied += 1;
      if (s.status === "timeout" || s.status === "error") slow += 1;
    });
    var text = "kwatch checked " + KW.plural(n, "thing", "things");
    if (read === n) return [n, text + (n === 1 ? " and could read it." : " and could read them all.")];
    var parts = [];
    if (read) parts.push("it read " + read);
    if (denied) parts.push("it isn't allowed to read " + denied);
    if (slow) parts.push(slow + " took too long or failed");
    var last = parts.pop();
    return [n, text + ": " + (parts.length ? parts.join(", ") + " and " : "") + last + "."];
  }

  function factList(facts) {
    var dl = KW.el("dl", "ev-facts");
    (facts || []).forEach(function (f) {
      dl.appendChild(KW.el("dt", "", f.label));
      dl.appendChild(KW.el("dd", "mono", f.value));
    });
    return dl;
  }

  function lineView(lines, matched, label) {
    var view = KW.el("div", "logs ev-log");
    view.setAttribute("role", "region");
    view.setAttribute("tabindex", "0");
    view.setAttribute("aria-label", label);
    var hit = {};
    (matched || []).forEach(function (i) { hit[i] = true; });
    (lines || []).forEach(function (l, i) {
      var row = KW.el("div", hit[i] ? "ev-line ev-hit" : "ev-line");
      var gutter = KW.el("span", "ev-gutter", hit[i] ? "matched" : String(i + 1));
      gutter.setAttribute("aria-hidden", hit[i] ? "false" : "true");
      row.appendChild(gutter);
      KW.hideMarks(row.appendChild(KW.el("span")), l);
      view.appendChild(row);
    });
    return view;
  }

  /* group builds one collapsible section with its heading button. */
  function group(id, title, b, open) {
    var sec = KW.el("section", "ev-sec");
    sec.id = "check-" + id;
    var head = KW.el("button", "ev-head");
    head.type = "button";
    head.setAttribute("aria-expanded", open ? "true" : "false");
    head.appendChild(KW.el("span", "ev-title", title));
    head.appendChild(KW.el("span", "chip " + b[1], b[0]));
    var body = KW.el("div", "ev-body");
    body.hidden = !open;
    head.onclick = function () {
      var on = head.getAttribute("aria-expanded") !== "true";
      head.setAttribute("aria-expanded", on ? "true" : "false");
      body.hidden = !on;
    };
    sec.appendChild(head);
    sec.appendChild(body);
    return [sec, body];
  }

  function checkSection(list) {
    var first = list[0];
    var worst = list.filter(function (s) { return s.status !== "ok"; })[0] || first;
    var open = worst.status === "denied" || worst.status === "timeout" || worst.status === "error" ||
      list.some(function (s) { return s.matched && s.matched.length; });
    var g = group(first.check, first.title, badge(worst), open);
    list.forEach(function (s) {
      if (s.subject && list.length > 1) g[1].appendChild(KW.el("h3", "ev-sub mono", s.subject));
      var note = bodyText(s);
      if (note) g[1].appendChild(KW.el("p", "ev-note", note));
      if (s.facts && s.facts.length) g[1].appendChild(factList(s.facts));
      if (s.lines && s.lines.length) {
        g[1].appendChild(lineView(s.lines, s.matched, first.title + " of " + (s.subject || "this object")));
      }
    });
    var at = readAt(list);
    if (at) g[1].appendChild(at);
    return g[0];
  }

  function correlation(root, c, titles) {
    var card = KW.el("section", "ev-card");
    var h = card.appendChild(KW.el("h3", "", "What the checks agree on"));
    h.title = "kwatch compared what each check read and kept what matches.";
    if (c.confirmed) card.appendChild(KW.el("p", "ev-confirmed", "Confirmed: " + c.confirmed + "."));
    var dl = KW.el("dl", "ev-facts");
    (c.facts || []).forEach(function (f) {
      dl.appendChild(KW.el("dt", "", f.label));
      var dd = KW.el("dd", "mono", f.value);
      var from = FROM[f.label];
      if (from === "logs" && titles.logs) {
        // The logs are in their own section, "Logs".
        var b = KW.el("button", "link ev-from", "Read from the logs");
        b.type = "button";
        b.onclick = function () { KW.openFold(root, "logs"); };
        dd.appendChild(document.createTextNode(" "));
        dd.appendChild(b);
      } else if (from && titles[from]) {
        var a = KW.el("a", "ev-from", "Read from " + titles[from]);
        a.href = "#check-" + from;
        dd.appendChild(document.createTextNode(" "));
        dd.appendChild(a);
      }
      dl.appendChild(dd);
    });
    card.appendChild(dl);
    return card;
  }

  /* chain lists Ingress, Service and pods with the first break marked. */
  function chain(sections) {
    var rows = [];
    sections.forEach(function (s) {
      if (s.status !== "ok") return;
      var facts = {};
      (s.facts || []).forEach(function (f) { facts[f.label] = f.value; });
      if (s.check === "ingress") rows.push(["Ingress", s.subject, facts["Class"] || ""]);
      if (s.check === "service") {
        rows.push(["Service", s.subject, (facts["Ready endpoints"] || "?") + " ready endpoints",
          facts["Ready endpoints"] === "0"]);
        rows.push(["Pods", (facts["Selected pods"] || "0") + " selected", "",
          facts["Selected pods"] === "0"]);
      }
    });
    if (!rows.length) return null;
    var box = KW.el("div", "ev-chain");
    var broke = false;
    rows.forEach(function (r) {
      var row = KW.el("div", "ev-node");
      row.appendChild(KW.el("span", "ev-kind", r[0]));
      row.appendChild(KW.el("span", "mono", r[1]));
      row.appendChild(KW.el("span", "", r[2]));
      if (r[3] && !broke) {
        broke = true;
        row.appendChild(KW.chip("c-page", "breaks here"));
      }
      box.appendChild(row);
    });
    return box;
  }

  function panel(root, inc) {
    var sections = inc.evidence;
    var box = KW.el("div", "ev");
    var sum = summary(sections);
    box.appendChild(KW.el("p", "ev-summary", sum[1]));
    var titles = {}, order = [], by = {};
    sections.forEach(function (s) {
      titles[s.check] = s.title;
      if (!by[s.check]) { by[s.check] = []; order.push(s.check); }
      by[s.check].push(s);
    });
    if (inc.correlation) box.appendChild(correlation(root, inc.correlation, titles));
    order.forEach(function (k) {
      // The logs a check read are shown in "Logs" (logs.js).
      if (k !== "logs") box.appendChild(checkSection(by[k]));
    });
    var cut = KW.cutCount(inc, "evidenceLines") + KW.cutCount(inc, "evidenceFacts") +
      KW.cutCount(inc, "evidenceSections");
    if (cut) {
      box.appendChild(KW.el("p", "ev-cap", "This evidence is too long to show in full: " +
        KW.plural(cut, "entry", "entries") + " not shown."));
    }
    return box;
  }

  function logClass(line) {
    var l = line.toLowerCase();
    if (/(fatal|panic|error|exception|failed|oomkilled)/.test(l)) return "e";
    if (/(warn|timeout|retry)/.test(l)) return "w";
    return "r";
  }

  /* logSources: what "Logs" can show. The lines kept when the issue
     started, then each container run a check read (Pro). */
  function logSources(inc) {
    var out = [], kept = inc.capturedLogLines || [];
    if (kept.length) {
      var cut = KW.cutCount(inc, "logLines");
      out.push({ key: "kept", label: "When it started", lines: kept, matched: [],
        cap: "Kept when this started. Secrets are removed." +
          (cut ? " " + KW.plural(cut, "earlier line", "earlier lines") + " not shown." : "") });
    }
    (inc.evidence || []).forEach(function (s) {
      if (s.check !== "logs") return;
      var lines = s.lines || [];
      out.push({ key: "run|" + s.subject + "|" + s.run, lines: lines, matched: s.matched || [],
        label: (s.subject || "Container") + ", " + (s.run === "previous" ?
          "run before the last restart" : "current run"),
        note: lines.length ? "" : bodyText(s) || "This container has written nothing yet.",
        cap: "Read from the container. Credentials are hidden.", at: s.readAt });
    });
    return out;
  }

  /* unreadable: "Logs" with nothing to show because kwatch may not read
     pod logs; the fix is in the Overview's coverage panel. */
  function unreadable(root, fold, box) {
    box.hidden = true;
    KW.$("[data-log-ctl]", root).hidden = true;
    KW.$("[data-copy-logs]", root).hidden = true;
    KW.$("[data-wrap-logs]", root).hidden = true;
    KW.$("[data-logcap]", root).textContent = "";
    var note = KW.$("[data-log-note]", root);
    note.hidden = false;
    note.textContent = "Crash output not shown: kwatch isn't allowed to read pod logs. ";
    var a = note.appendChild(KW.el("a", "ulink", "How to allow it"));
    a.href = "/#cov";
    a.dataset.link = "";
  }

  /* KW.logs fills "Logs": one view with one set of controls. "Show"
     picks the source (kept on redraws); a line the checks matched is
     marked. */
  KW.logs = function (root, inc) {
    var fold = KW.$('[data-fold="logs"]', root), box = KW.$("[data-logs]", root);
    var list = logSources(inc);
    var keep = box.scrollTop, keepX = box.scrollLeft, was = fold.dataset.pick;
    box.textContent = "";
    fold.hidden = !list.length && !inc.logsNotReadable;
    if (fold.hidden) return;
    if (!list.length) return unreadable(root, fold, box);
    var src = list.filter(function (x) { return x.key === was; })[0] ||
      list.filter(function (x) { return x.matched.length; })[0] || list[0];
    fold.dataset.pick = src.key;
    var pick = KW.$("[data-log-pick]", root);
    var keys = list.map(function (x) { return x.key; }).join("\n");
    if (pick.dataset.keys !== keys) {
      pick.dataset.keys = keys;
      pick.textContent = "";
      list.forEach(function (x) {
        var o = pick.appendChild(KW.el("option", "", x.label));
        o.value = x.key;
      });
    }
    pick.value = src.key;
    pick.onchange = function () {
      fold.dataset.pick = pick.value;
      KW.logs(root, inc);
    };
    KW.$("[data-log-ctl]", root).hidden = list.length < 2;
    KW.$("[data-log-n]", root).textContent = KW.plural(src.lines.length, "line", "lines");
    var hit = {};
    src.matched.forEach(function (i) { hit[i] = true; });
    src.lines.forEach(function (l, i) {
      var row = box.appendChild(KW.el("div", logClass(l) + (hit[i] ? " ev-hit" : "")));
      KW.hideMarks(row, l);
      if (hit[i]) row.title = "The line kwatch matched";
    });
    box.hidden = !src.lines.length;
    var note = KW.$("[data-log-note]", root);
    note.hidden = !src.note;
    note.textContent = src.note || "";
    if (was === src.key) {
      box.scrollTop = keep;
      box.scrollLeft = keepX;
    }
    var cap = KW.$("[data-logcap]", root);
    cap.textContent = src.cap;
    if (src.at && KW.valid(src.at)) {
      cap.appendChild(document.createTextNode(" Read at "));
      cap.appendChild(KW.hm(src.at));
      cap.appendChild(document.createTextNode("."));
    }
    var copy = KW.$("[data-copy-logs]", root);
    copy.hidden = !src.lines.length;
    copy.onclick = function () { KW.copy(copy, src.lines.join("\n"), "Copy all"); };
    // Long lines scroll sideways by default; wrapping reads easier.
    var wrap = KW.$("[data-wrap-logs]", root);
    wrap.hidden = !src.lines.length;
    wrap.onclick = function () {
      var on = box.classList.toggle("wrap");
      wrap.setAttribute("aria-pressed", on ? "true" : "false");
    };
  };

  /* KW.evidence fills the evidence under "Why kwatch thinks this" and
     the chain in "Affected objects". Without evidence (Community) both
     stay out; KW.objects shows the section for its own table. ?tab=evidence, the old link to the evidence, opens
     "Why kwatch thinks this". */
  KW.evidence = function (root, inc) {
    var host = KW.$("[data-evidence]", root);
    var related = KW.$('[data-fold="related"]', root);
    var has = !!(inc.evidence && inc.evidence.length);
    var box = KW.$("[data-related]", root);
    host.hidden = !has;
    host.textContent = "";
    box.textContent = "";
    related.hidden = true;
    if (!has) return;
    host.appendChild(panel(root, inc));
    var side = chain(inc.evidence);
    if (side) {
      box.appendChild(side);
      related.hidden = false;
    }
    if (!root.dataset.opened &&
        new URLSearchParams(location.search).get("tab") === "evidence") {
      KW.$('[data-fold="why"]', root).open = true;
    }
    root.dataset.opened = "1";
  };
})();
