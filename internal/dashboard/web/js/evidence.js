/* kwatch dashboard: the Evidence tab of an issue (Pro). Every string from
   the API is set with textContent; log text is never interpreted. */
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

  /* readAt is the "Read at 13:44:02" footer of a section, or null. */
  function readAt(list) {
    var iso = list.map(function (s) { return s.readAt; }).filter(Boolean).sort().pop();
    if (!iso || !KW.valid(iso)) return null;
    var d = new Date(iso);
    var t = KW.el("time", "", "");
    t.dateTime = iso;
    t.textContent = KW.hhmm(d) + ":" + ("0" + (KW.utc ? d.getUTCSeconds() : d.getSeconds())).slice(-2) +
      (KW.utc ? " UTC" : "");
    var p = KW.el("p", "ev-cap", "Read at ");
    p.appendChild(t);
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
    var text = KW.plural(n, "check", "checks") + " ran.";
    if (read === n) return [n, text + " All read."];
    var parts = [];
    if (read) parts.push(read + " read");
    if (denied) parts.push(denied + " not allowed");
    if (slow) parts.push(slow + " timed out or could not be read");
    return [n, text + " " + parts.join(", ") + "."];
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
      row.appendChild(KW.el("span", "", l));
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

  /* logSection shows one container and run at a time. */
  function logSection(list) {
    var worst = list.filter(function (s) { return s.status !== "ok"; })[0] || list[0];
    var g = group("logs", "Logs", badge(worst), true);
    var containers = [];
    list.forEach(function (s) { if (containers.indexOf(s.subject) < 0) containers.push(s.subject); });
    var controls = KW.el("div", "ev-controls");
    var pick = KW.el("select");
    pick.setAttribute("aria-label", "Container");
    containers.forEach(function (c) { var o = KW.el("option", "", c); o.value = c; pick.appendChild(o); });
    var runs = KW.el("div", "ev-runs");
    runs.setAttribute("role", "radiogroup");
    runs.setAttribute("aria-label", "Run");
    var run = list.some(function (s) { return s.run === "previous"; }) ? "previous" : "current";
    var count = KW.el("span", "ev-count");
    var copy = KW.el("button", "btn btn-sm", "Copy all");
    copy.type = "button";
    var view = KW.el("div");
    function show() {
      var s = list.filter(function (x) { return x.subject === pick.value && x.run === run; })[0];
      view.textContent = "";
      var lines = (s && s.lines) || [];
      if (!s || lines.length === 0) {
        view.appendChild(KW.el("p", "ev-note", s ? bodyText(s) ||
          "This container has written nothing yet." : "No " + run + " run read."));
      } else {
        view.appendChild(lineView(lines, s.matched, "Log of container " + s.subject + ", " + run + " run"));
      }
      count.textContent = "Showing " + KW.plural(lines.length, "line", "lines");
      copy.onclick = function () { KW.copy(copy, lines.join("\n"), "Copy all"); };
      KW.$$("button", runs).forEach(function (b) {
        b.setAttribute("aria-checked", b.dataset.run === run ? "true" : "false");
      });
    }
    [["previous", "Previous run"], ["current", "Current run"]].forEach(function (r) {
      var b = KW.el("button", "btn btn-sm", r[1]);
      b.type = "button";
      b.dataset.run = r[0];
      b.setAttribute("role", "radio");
      b.onclick = function () { run = r[0]; show(); };
      runs.appendChild(b);
    });
    pick.onchange = show;
    [pick, runs, count, copy].forEach(function (e) { controls.appendChild(e); });
    g[1].appendChild(controls);
    g[1].appendChild(view);
    g[1].appendChild(KW.el("p", "ev-cap", "Credentials are hidden in these lines."));
    var at = readAt(list);
    if (at) g[1].appendChild(at);
    show();
    return g[0];
  }

  function correlation(c, titles) {
    var card = KW.el("section", "ev-card");
    card.appendChild(KW.el("h2", "", "Correlation"));
    if (c.confirmed) card.appendChild(KW.el("p", "ev-confirmed", "Confirmed: " + c.confirmed + "."));
    var dl = KW.el("dl", "ev-facts");
    (c.facts || []).forEach(function (f) {
      dl.appendChild(KW.el("dt", "", f.label));
      var dd = KW.el("dd", "mono", f.value);
      var from = FROM[f.label];
      if (from && titles[from]) {
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
    var box = KW.el("aside", "ev-chain");
    box.setAttribute("aria-label", "Related objects");
    box.appendChild(KW.el("h2", "", "Related"));
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

  function panel(inc) {
    var sections = inc.evidence;
    var root = KW.el("div", "ev");
    var sum = summary(sections);
    root.appendChild(KW.el("p", "ev-summary", sum[1]));
    var titles = {}, order = [], by = {};
    sections.forEach(function (s) {
      titles[s.check] = s.title;
      if (!by[s.check]) { by[s.check] = []; order.push(s.check); }
      by[s.check].push(s);
    });
    var cols = KW.el("div", "ev-cols");
    var main = KW.el("div", "ev-main");
    if (inc.correlation) main.appendChild(correlation(inc.correlation, titles));
    var side = chain(sections);
    order.forEach(function (k) { main.appendChild(k === "logs" ? logSection(by[k]) : checkSection(by[k])); });
    var fix = KW.el("section", "ev-sec");
    fix.appendChild(KW.el("h2", "", "Fix"));
    (inc.fixSteps || []).forEach(function (s) {
      if (s.text) fix.appendChild(KW.el("p", "", s.text));
      if (s.command) fix.appendChild(KW.el("pre", "logs mono", s.command));
    });
    fix.appendChild(KW.el("p", "ev-cap", "This page is read-only. To change anything, use the commands under Fix."));
    main.appendChild(fix);
    if (side) cols.appendChild(side);
    cols.appendChild(main);
    root.appendChild(cols);
    return [sum[0], root];
  }

  function wantEvidence() {
    return new URLSearchParams(location.search).get("tab") === "evidence";
  }

  /* KW.evidence fills the tab strip and the Evidence panel. Without
     evidence (Community) the strip stays hidden and the page is as
     before. */
  KW.evidence = function (root, inc) {
    var strip = KW.$("[data-tabs]", root);
    var story = KW.$("[data-story]", root);
    var host = KW.$("[data-evidence]", root);
    var has = inc.evidence && inc.evidence.length;
    strip.hidden = !has;
    if (!has) { host.hidden = true; story.hidden = false; return; }
    var built = panel(inc);
    host.textContent = "";
    host.appendChild(KW.el("h2", "sr-only", "Evidence"));
    host.appendChild(built[1]);
    var tab = KW.$("[data-tab-evidence]", root);
    KW.$("span", tab).textContent = String(built[0]);
    tab.setAttribute("aria-label", "Evidence, " + KW.plural(built[0], "check", "checks"));
    function select(evidence) {
      host.hidden = !evidence;
      story.hidden = evidence;
      KW.$$("[role=tab]", strip).forEach(function (t) {
        var on = (t.dataset.tab === "evidence") === evidence;
        t.setAttribute("aria-selected", on ? "true" : "false");
        t.tabIndex = on ? 0 : -1;
      });
    }
    KW.$$("[role=tab]", strip).forEach(function (t) {
      t.onclick = function (e) {
        e.preventDefault();
        var evidence = t.dataset.tab === "evidence";
        history.replaceState(null, "", evidence ? "?tab=evidence" : location.pathname);
        select(evidence);
      };
      t.onkeydown = function (e) {
        if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
        var other = KW.$("[role=tab]:not([data-tab=" + t.dataset.tab + "])", strip);
        other.focus();
        other.click();
      };
    });
    select(wantEvidence());
  };
})();
