/* kwatch dashboard: the issue page. On top, always shown, the summary
   from the API's `summary`: what is wrong, why (and how sure kwatch
   is), the proof, what to do and who was told, with a stepper of where
   the issue is. Below it, tabs (incpanel.js) hold everything else, one
   section each. The template's folded sections are turned into those
   sections once, by layout; the modules that fill them keep their
   data- hooks. */
(function () {
  "use strict";
  var KW = window.KW;
  var NS = "http://www.w3.org/2000/svg";

  /* ISSUE_TABS: key, label, shortcut key and the sections it holds. */
  KW.ISSUE_TABS = [
    ["timeline", "Timeline", "t", ["timeline", "changes"]],
    ["evidence", "Evidence", "e", ["probes", "waiting", "logs", "containers", "warnings", "node"]],
    ["impact", "Impact", "i", ["blast", "related", "traffic", "others"]],
    ["alerts", "Alerts", "a", ["alert", "told"]],
    ["details", "Details", "d", ["why", "fix", "similar", "seen", "more"]]
  ];

  /* KW.issueSection makes an empty section of a tab: a heading and a
     body (.fold-body), hidden until its module fills it. */
  KW.issueSection = function (name, title) {
    var sec = KW.el("section", "sec");
    sec.dataset.fold = name;
    sec.hidden = true;
    sec.appendChild(KW.el("h3", "sec-h", title));
    sec.appendChild(KW.el("div", "fold-body"));
    return sec;
  };

  /* section turns a folded <details> of the template into a section,
     keeping its data- hooks and its heading's. */
  function section(fold) {
    var sec = KW.el("section", "sec");
    sec.dataset.fold = fold.dataset.fold;
    sec.hidden = fold.hidden;
    var sum = KW.$("summary", fold), h = sec.appendChild(KW.el("h3", "sec-h"));
    Object.keys(sum.dataset).forEach(function (k) { h.dataset[k] = sum.dataset[k]; });
    while (sum.firstChild) h.appendChild(sum.firstChild);
    fold.removeChild(sum);
    while (fold.firstChild) sec.appendChild(fold.firstChild);
    fold.parentNode.replaceChild(sec, fold);
  }

  function hook(tag, cls, name) { var n = KW.el(tag, cls); n.dataset[name] = ""; return n; }

  /* ROWS: the summary's lines, label first. */
  var ROWS = [["what", "What's wrong"], ["why", "Why"], ["proof", "Proof"],
    ["next", "What to do"], ["told", "Who was told"]];

  /* top builds the summary card from the template's, keeping its
     chips, title, copy buttons and notes. */
  function top(old) {
    var sum = KW.el("section", "d-sum ds");
    sum.setAttribute("aria-labelledby", "d-title");
    var head = sum.appendChild(KW.el("div", "ds-top"));
    head.appendChild(KW.$("[data-chips]", old));
    head.appendChild(KW.$(".d-acts", old));
    sum.appendChild(KW.$("[data-title]", old));
    sum.appendChild(hook("p", "ds-where", "dsWhere"));
    var stage = sum.appendChild(KW.el("div", "ds-stage"));
    var ol = stage.appendChild(hook("ol", "ds-steps", "dsSteps"));
    ol.setAttribute("aria-label", "Where this issue is");
    stage.appendChild(hook("p", "ds-stage-note", "dsNote"));
    var notes = sum.appendChild(KW.el("div", "ds-notes"));
    notes.appendChild(hook("ul", "ds-dyn", "dsDyn"));
    notes.appendChild(KW.$("[data-caused]", old));
    notes.appendChild(hook("div", "d-moved", "moved")).hidden = true;
    notes.appendChild(KW.$("[data-looked]", old));
    notes.appendChild(hook("p", "ev-cap d-short", "short")).hidden = true;
    var dl = sum.appendChild(KW.el("dl", "ds-grid"));
    ROWS.forEach(function (r) {
      var div = dl.appendChild(KW.el("div", "ds-row ds-r-" + r[0]));
      div.dataset.dsRow = r[0];
      div.appendChild(KW.el("dt", "", r[1]));
      div.appendChild(KW.el("dd"));
    });
    sum.appendChild(hook("nav", "ds-more", "dsMore")).hidden = true;
    return sum;
  }

  /* layout turns the template into the summary and the tab panels. */
  function layout(root) {
    var detail = KW.$(".detail", root), old = KW.$(".d-sum", root);
    var sum = top(old);
    KW.$(".ds-stage", sum).appendChild(KW.$("[data-why-open]", root));
    var blast = KW.issueSection("blast", "Who it hits");
    KW.$(".fold-body", blast).appendChild(hook("div", "bl-chips", "blastChips"));
    KW.$(".fold-body", blast).appendChild(KW.$("[data-blast]", old));
    var similar = KW.issueSection("similar", "Same error elsewhere");
    KW.$(".fold-body", similar).appendChild(KW.$("[data-similar]", old));
    old.parentNode.replaceChild(sum, old);
    detail.appendChild(blast);
    detail.appendChild(similar);
    KW.$("[data-alert-box]", root).dataset.fold = "alert";
    KW.$$("details.fold", detail).forEach(section);
    var panels = detail.appendChild(KW.el("div", "d-panels"));
    KW.ISSUE_TABS.forEach(function (t) {
      var p = panels.appendChild(KW.el("div", "d-panel"));
      p.dataset.panel = t[0];
      p.setAttribute("role", "tabpanel");
      p.tabIndex = 0;
      p.appendChild(KW.el("h2", "sr-only", t[1]));
      p.appendChild(KW.el("p", "tab-empty")).hidden = true;
      t[3].forEach(function (f) { p.appendChild(KW.$('[data-fold="' + f + '"]', detail)); });
    });
  }

  /* stopped: the banner of an issue kwatch stopped tracking: it is
     not fixed. A workload also says what was still wrong. */
  function stopped(inc) {
    var text = "kwatch stopped tracking this after " +
      KW.lasted(inc.firstSeen, inc.resolvedAt) + "; it is still not healthy, so check it yourself.";
    if (inc.workloadKind && inc.workload) {
      text += inc.stillCrashing ? " A container still keeps crashing." : " It is still short of replicas.";
    }
    return text;
  }

  function sum(inc) { return inc.summary || {}; }

  function sep(p) { p.appendChild(KW.el("span", "ds-sep", " · ")); }

  /* header: severity, then "shop · deployment/api · for 40 min". */
  function header(root, inc) {
    var resolved = inc.state === "resolved", chips = KW.$("[data-chips]", root);
    chips.textContent = "";
    chips.appendChild(KW.severityChip(inc.severity, resolved ? "Was: " : ""));
    if (KW.isUntold(inc)) chips.appendChild(KW.untoldMark());
    var h1 = KW.$("[data-title]", root);
    h1.textContent = inc.title;
    KW.clampTitle(h1);
    var p = KW.$("[data-ds-where]", root), o = KW.obj(inc.workloadKind, inc.namespace, inc.workload);
    p.textContent = "";
    p.appendChild(KW.el("span", "mono", inc.namespace || "whole cluster"));
    if (o) { sep(p); p.appendChild(KW.el("span", "mono", o)); }
    sep(p);
    if (resolved && KW.valid(inc.resolvedAt)) {
      p.appendChild(KW.el("b", "", "lasted " + KW.lasted(inc.firstSeen, inc.resolvedAt)));
      p.appendChild(document.createTextNode(inc.stoppedTracking ? ", stopped tracking " : ", resolved "));
      p.appendChild(KW.hm(inc.resolvedAt));
      return;
    }
    var long = p.appendChild(KW.el("b", "", KW.forLong(inc.firstSeen)));
    long.title = "Started " + KW.full(inc.firstSeen);
    p.appendChild(document.createTextNode(" (since "));
    p.appendChild(KW.hm(inc.firstSeen));
    p.appendChild(document.createTextNode(")"));
    if (inc.repeatCount > 0) { sep(p); p.appendChild(document.createTextNode(KW.failedAgain(inc.repeatCount).toLowerCase())); }
  }

  function row(root, key, show) {
    var r = KW.$('[data-ds-row="' + key + '"]', root), dd = KW.$("dd", r);
    dd.textContent = "";
    r.hidden = !show;
    return dd;
  }

  var CONF = {
    high: [3, "High confidence", "The checks agree and no other cause competes."],
    likely: [2, "Likely", "Most of what kwatch read points here."],
    possible: [1, "Possible", "Some signs point here. Check the proof."]
  };

  /* meter: three steps and a word, never color alone. */
  function meter(c) {
    var m = KW.el("span", "ds-meter m-" + c), n = CONF[c][0];
    m.title = CONF[c][2];
    var bars = m.appendChild(KW.el("span", "mt-bars"));
    bars.setAttribute("role", "img");
    bars.setAttribute("aria-label", "Confidence " + n + " of 3");
    for (var i = 0; i < 3; i++) bars.appendChild(KW.el("span", i < n ? "mt on" : "mt"));
    m.appendChild(KW.el("span", "mt-t", CONF[c][1]));
    return m;
  }

  function why(root, inc) {
    var w = sum(inc).why || {}, text = w.text || inc.rootCause || "";
    var known = w.causeKnown === undefined ? !!text : !!w.causeKnown && !!text;
    var dd = row(root, "why", true);
    if (known) {
      KW.hideMarks(dd.appendChild(KW.el("p", "ds-why-t")), text);
      if (CONF[w.confidence]) dd.appendChild(meter(w.confidence));
      return;
    }
    dd.appendChild(KW.el("p", "ds-why-t ds-unknown", inc.state === "resolved" ? "Cause not found." : "Cause not found yet."));
    var more = inc.state === "settling" ? "kwatch is still collecting what failed around it." : "";
    if (KW.checkedText) more = [more, KW.checkedText(inc)].filter(Boolean).join(" ");
    if (more) dd.appendChild(KW.el("p", "ds-sub", more));
  }

  var KIND = { probe: "Health check", log: "Log", exit: "Exit", scheduler: "Scheduler",
    endpoints: "Endpoints", change: "Change", node: "Node", event: "Event" };
  var ICON = { probe: "M1 8h3l2-5 3 10 2-5h4", log: "M3 4h10M3 8h10M3 12h6", exit: "M4 4l8 8M12 4l-8 8",
    scheduler: "M8 4v4l3 2M14 8A6 6 0 1 1 2 8a6 6 0 0 1 12 0", endpoints: "M8 1v5M8 10v5M1 8h5M10 8h5",
    change: "M8 2l6 12H2z", node: "M2 3h12v4H2zM2 9h12v4H2z", event: "M8 2l7 12H1zM8 6v4M8 11v2" };

  function icon(kind) {
    var s = document.createElementNS(NS, "svg");
    s.setAttribute("class", "ds-ico");
    s.setAttribute("viewBox", "0 0 16 16");
    s.setAttribute("aria-hidden", "true");
    var p = document.createElementNS(NS, "path");
    p.setAttribute("d", ICON[kind] || ICON.event);
    s.appendChild(p);
    return s;
  }

  /* raw: the exact value in monospace, with its own Copy. */
  function raw(text) {
    var box = KW.el("div", "ds-raw");
    var code = box.appendChild(KW.el("code", "ds-raw-t"));
    code.tabIndex = 0;
    code.title = "The exact text. Click or press Enter to show it all.";
    code.setAttribute("aria-label", text);
    KW.hideMarks(code, text);
    function grow(e) {
      if (e.type === "keydown" && e.key !== "Enter") return;
      if (KW.selecting(box)) return;
      code.classList.toggle("is-open");
    }
    code.addEventListener("click", grow);
    code.addEventListener("keydown", grow);
    var b = box.appendChild(KW.el("button", "btn btn-sm ds-copy", "Copy"));
    b.type = "button";
    b.title = "Copy the exact text";
    b.addEventListener("click", function () { KW.copy(b, text, "Copy"); });
    return box;
  }

  /* what: "What's wrong". When it only repeats the title (ignoring
     case and punctuation), the first probe, log or exit proof that says
     something else takes its place and leaves the Proof row; with none,
     the row hides and every proof stays in Proof. Returns that proof's
     index. */
  var LEADS = { probe: 1, log: 1, exit: 1 };
  function what(root, inc) {
    function bare(x) { return String(x || "").toLowerCase().replace(/[^a-z0-9]+/g, " ").trim(); }
    var w = sum(inc).what && KW.problemTitle(sum(inc).what, inc), list = sum(inc).proof || [], title = bare(inc.title);
    var at = bare(w) && bare(w) !== title ? -2 : list.findIndex(function (p) {
      return LEADS[p.kind] && bare(p.text) && bare(p.text) !== title;
    });
    if (at >= 0) w = list[at].text;
    var dd = row(root, "what", at !== -1);
    if (at === -1) return -1;
    KW.hideMarks(dd.appendChild(KW.el("p", "ds-what")), w);
    if (at >= 0 && list[at].detail) dd.appendChild(raw(list[at].detail));
    return at;
  }

  function proof(root, inc, skip) {
    var list = (sum(inc).proof || []).filter(function (p, i) { return i !== skip; });
    var cut = KW.cutCount(inc, "summaryProof");
    var dd = row(root, "proof", list.length > 0 || cut > 0);
    var ul = dd.appendChild(KW.el("ul", "ds-proof"));
    list.forEach(function (p) {
      var li = ul.appendChild(KW.el("li", "ds-pf"));
      li.appendChild(icon(p.kind));
      var body = li.appendChild(KW.el("div", "ds-pf-b"));
      var head = body.appendChild(KW.el("p", "ds-pf-h"));
      head.appendChild(KW.el("span", "ds-kind", KIND[p.kind] || p.kind));
      KW.hideMarks(head.appendChild(KW.el("span")), p.text);
      if (p.count > 1) head.appendChild(KW.el("span", "ds-n", "seen " + KW.plural(p.count, "time", "times")));
      if (KW.valid(p.at)) head.appendChild(KW.el("span", "ds-at")).appendChild(KW.hm(p.at));
      if (p.detail) body.appendChild(raw(p.detail));
    });
    if (cut) dd.appendChild(KW.el("p", "ev-cap", KW.plural(cut, "more proof", "more proofs") + " not shown. See Evidence."));
  }

  function next(root, inc) {
    var steps = inc.fixSteps || [], n = sum(inc).next;
    if (!n && steps.length) n = { text: steps[0].text, command: steps[0].command, more: steps.length - 1 };
    var dd = row(root, "next", !!n || !!inc.docsLink);
    if (!n) return KW.runbook(dd.appendChild(KW.el("p")), inc.docsLink);
    if (n.text) KW.hideMarks(dd.appendChild(KW.el("p", "ds-next-t")), n.text);
    if (n.command) dd.appendChild(KW.cmd(n.command));
    var more = n.more || 0;
    if (!more && !KW.cutCount(inc, "summaryNext") && !inc.docsLink) return;
    var b = dd.appendChild(KW.el("button", "link ds-steps-b", more ? KW.plural(more, "more step", "more steps") + " →" :
      "All steps and the runbook →"));
    b.type = "button";
    b.addEventListener("click", function () { KW.openFold(root, "fix"); });
  }

  /* KW.summaryTop draws the summary card. */
  function summaryTop(root, inc) {
    header(root, inc);
    KW.issueStepper(root, inc);
    var used = what(root, inc);
    why(root, inc);
    proof(root, inc, used);
    next(root, inc);
    KW.toldSummary(row(root, "told", true), inc, root);
    KW.summaryNotes(root, inc);
  }

  /* KW.renderIncident fills a tpl-detail clone (root) with an issue.
     The issue page and the preview drawer both draw with it. */
  KW.renderIncident = function (root, inc) {
    if (!root.dataset.laid) {
      layout(root);
      root.dataset.laid = "1";
    }
    // First: what is new since the last visit marks the parts below.
    KW.lastLook(root, inc);
    var banner = KW.$("[data-banner]", root), stop = inc.state === "resolved" && inc.stoppedTracking;
    banner.hidden = !stop;
    if (stop) {
      banner.className = "notice d-banner n-degraded";
      KW.$("b", banner).textContent = "Stopped tracking: not fixed";
      KW.$("p", banner).textContent = stopped(inc);
    }
    summaryTop(root, inc);
    KW.similar(root, inc);
    KW.alertBox(root, inc);
    var copyLink = KW.$("[data-copy-link]", root);
    copyLink.onclick = function () {
      KW.copy(copyLink, location.origin + "/issues/" + encodeURIComponent(inc.id), "Copy link");
    };
    KW.$("[data-cause]", root).textContent = inc.rootCause || (inc.state === "settling" ?
      "Not sure yet. kwatch is still collecting what failed around it." :
      "kwatch has no single cause for this.");
    KW.fixSteps(root, inc);
    KW.logs(root, inc);
    KW.timeline(KW.$("[data-timeline]", root), KW.$("[data-tl-more]", root), inc);
    KW.facts(KW.$("[data-facts]", root), inc);
    KW.evidence(root, inc);
    KW.objects(root, inc);
    KW.context(root, inc);
    KW.links(root, inc);
    KW.usageChart(root, inc);
    KW.shortNote(root, inc);
    KW.notesRow(root);
  };

  /* KW.detailView is the issue page, /issues/<id>: the issue
     (incpanel.js), its tab title and the link back to the table. */
  KW.detailView = function (main, r) {
    var panel = KW.incidentPanel(main, r.id, {
      page: true,
      scrollBack: KW.takeScroll(),
      each: KW.refreshBadge,
      notFound: function () { KW.issueNotFound(main); },
      drawn: function (root, inc) {
        KW.title(inc.title);
        KW.setNav("issues");
        var back = KW.$("[data-back]", root);
        back.setAttribute("href", KW.lastList !== "/issues" ? KW.lastList :
          inc.state === "resolved" ? "/issues?status=resolved" : "/issues");
        KW.$("span", back).textContent = "Issues";
      }
    });
    return { stop: panel.stop };
  };
})();
