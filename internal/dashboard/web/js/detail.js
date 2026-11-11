/* kwatch dashboard: the issue page. On top, always shown, the header
   card: level and state, the short title, where and since when, the
   whole title when the heading had to shorten it, and the copy
   buttons. Under it, tabs (incpanel.js) hold everything else: Summary
   (the API's `summary` as plain questions, What's wrong?, Why? and
   What should I do? (incsum.js), and What kwatch found), then one
   section each in the other tabs; Who was alerted? opens the Alerts
   tab. The drawer keeps the questions in the card. The template's
   folded sections are turned into those sections once, by layout; the
   modules that fill them keep their data- hooks. */
(function () {
  "use strict";
  var KW = window.KW;
  var T = KW.TERMS;

  /* ISSUE_TABS: key, label, shortcut key and the sections it holds. */
  KW.ISSUE_TABS = [
    ["summary", "Summary", "s", []],
    ["timeline", "Timeline", "t", ["changes", "timeline"]],
    ["evidence", T.proof, "p", ["probes", "waiting", "logs", "containers", "warnings", "node", "why"]],
    ["impact", "Impact", "i", ["leftovers", "blast", "others", "traffic", "related", "map"]],
    ["alerts", "Alerts", "a", ["alert", "told"]],
    ["details", "Details", "d", ["seen", "similar", "more", "fix"]]
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

  /* ROWS: the summary's questions; Who was alerted? is apart, for
     the Alerts tab. */
  var ROWS = [["what", T.ask.what], ["why", T.ask.why], ["next", T.ask.todo]];

  function rows(dl, list) {
    list.forEach(function (r) {
      var div = dl.appendChild(KW.el("div", "ds-row ds-r-" + r[0]));
      div.dataset.dsRow = r[0];
      div.appendChild(KW.el("dt", "", r[1]));
      div.appendChild(KW.el("dd"));
    });
    return dl;
  }

  /* top builds the summary card from the template's, keeping its
     chips, title, copy buttons and notes. The header: the level and
     state chips with the copy buttons, the short title, one line of
     where and since when, and the whole title when it was cut. Then
     the summary's body (.ds-body: the stepper, the notes and the
     questions) and Who was alerted? (.ds-alerted), which the issue
     page moves into the Summary and Alerts tabs (incpanel.js). */
  function top(old) {
    var sum = KW.el("section", "d-sum ds");
    sum.setAttribute("aria-labelledby", "d-title");
    var head = sum.appendChild(KW.el("header", "ds-head"));
    var bar = head.appendChild(KW.el("div", "ds-top"));
    bar.appendChild(KW.$("[data-chips]", old));
    bar.appendChild(KW.$(".d-acts", old));
    head.appendChild(KW.$("[data-title]", old));
    head.appendChild(hook("ul", "ds-where", "dsWhere"));
    head.appendChild(hook("p", "ds-lead", "dsLead")).hidden = true;
    var body = sum.appendChild(KW.el("div", "ds ds-body"));
    var stage = body.appendChild(KW.el("div", "ds-stage"));
    var notes = body.appendChild(KW.el("div", "ds-notes"));
    notes.appendChild(hook("ul", "ds-dyn", "dsDyn"));
    notes.appendChild(KW.$("[data-caused]", old));
    notes.appendChild(hook("div", "d-moved", "moved")).hidden = true;
    notes.appendChild(KW.$("[data-looked]", old));
    notes.appendChild(hook("p", "ev-cap d-short", "short")).hidden = true;
    rows(body.appendChild(KW.el("dl", "ds-grid")), ROWS);
    body.appendChild(hook("nav", "ds-more", "dsMore")).hidden = true;
    rows(sum.appendChild(KW.el("dl", "ds-grid ds-alerted")),
      [["told", T.ask.told]]);
    return sum;
  }

  /* layout turns the template into the summary and the tab panels. */
  function layout(root) {
    var detail = KW.$(".detail", root), old = KW.$(".d-sum", root);
    var sum = top(old);
    KW.$(".ds-stage", sum).appendChild(KW.$("[data-why-open]", root));
    var blast = KW.issueSection("blast", "Who it hits");
    KW.$(".fold-body", blast).appendChild(KW.$("[data-blast]", old));
    var similar = KW.issueSection("similar", "Is it happening anywhere else?");
    KW.$(".fold-body", similar).appendChild(KW.$("[data-similar]", old));
    old.parentNode.replaceChild(sum, old);
    detail.appendChild(KW.issueSection("leftovers", T.paging.leftHead));
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

  /* heading: the whole title. `title` is the short one lists use and
     ends in "…" when the sentence is long; `fullTitle` is the same
     title uncut (older answers have only `title`). */
  function heading(inc) {
    return inc.fullTitle ? KW.problemTitle(inc.fullTitle, inc) : inc.title;
  }

  /* KW.issueHeading: the heading of an issue: the short title lists
     use (KW.shortTitle, the Overview's rule). The whole sentence is the
     description line under it. */
  KW.issueHeading = function (inc) { return KW.shortTitle(heading(inc), inc); };

  /* item adds one part of the where line; CSS puts a dot between. */
  function item(ul, node, cls) {
    var li = ul.appendChild(KW.el("li", cls || ""));
    li.appendChild(typeof node === "string" ? document.createTextNode(node) : node);
    return li;
  }

  /* since: "for 40 min (since 03:28)", or "lasted 12 min, resolved
     14:02" on a resolved issue. */
  function since(inc) {
    var span = KW.el("span");
    function add(text) { span.appendChild(document.createTextNode(text)); }
    if (inc.state === "resolved" && KW.valid(inc.resolvedAt)) {
      add("lasted " + KW.lasted(inc.firstSeen, inc.resolvedAt) + ", resolved ");
      span.appendChild(KW.hm(inc.resolvedAt));
      return span;
    }
    var long = inc.state !== "settling" && KW.forLong(inc.firstSeen);
    if (!KW.valid(inc.firstSeen)) return long ? KW.el("span", "", long) : null;
    add(long && long !== "now" ? long + " (since " : "since ");
    span.appendChild(KW.hm(inc.firstSeen));
    if (long && long !== "now") add(")");
    return span;
  }

  /* header: the alert level and the state as chips, the short title,
     and one line: "shop · deployment/api · for
     40 min (since 03:28) · failed again 2 times", names wrapping at /
     and . The state's details (users still served, …) follow it. */
  function header(root, inc) {
    var chips = KW.$("[data-chips]", root);
    chips.textContent = "";
    // "Was Urgent" only for a resolved issue people were alerted about;
    // one that settled on its own shows its level greyed beside
    // "Settled on its own".
    chips.appendChild(KW.isUntold(inc) ? KW.settledChip(inc.severity) :
      KW.severityChip(inc.severity, inc.state === "resolved" ? "Was " : ""));
    chips.appendChild(KW.issueStateChip(inc));
    if (KW.isUntold(inc)) chips.appendChild(KW.untoldMark());
    KW.levelLine(chips.parentNode, inc);
    var h1 = KW.$("[data-title]", root);
    h1.textContent = KW.issueHeading(inc);
    h1.title = heading(inc);
    var ul = KW.$("[data-ds-where]", root), o = KW.obj(inc.workloadKind, inc.namespace, inc.workload);
    ul.textContent = "";
    item(ul, KW.wrapName(inc.namespace || "whole cluster"), "ds-ns");
    if (o) item(ul, KW.wrapName(o), "ds-obj");
    var when = since(inc);
    if (when) item(ul, when, "ds-when");
    if (inc.state !== "resolved" && inc.repeatCount > 0) {
      item(ul, KW.failedAgain(inc.repeatCount).toLowerCase());
    }
    KW.issueStatus(root, inc).forEach(function (b) { item(ul, b, "ds-impact"); });
  }

  /* KW.summaryRow empties row key, sets its question and shows it or
     not; it returns the row's body. */
  KW.summaryRow = function (root, key, label, show) {
    var r = KW.$('[data-ds-row="' + key + '"]', root), dd = KW.$("dd", r);
    dd.textContent = "";
    KW.$("dt", r).textContent = label;
    r.hidden = !show;
    return dd;
  };

  function bare(x) { return String(x || "").toLowerCase().replace(/[^a-z0-9]+/g, " ").trim(); }

  /* core: a title's words without "in <namespace>" (and its cluster
     note), which the line under the title already names. */
  function core(x, inc) {
    var s = String(x || ""), ns = inc.namespace;
    if (ns) {
      var esc = ns.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      s = s.replace(new RegExp(" in " + esc + "( \\([^)]*\\))?(?=[ :.;,]|$)", "g"), "");
    }
    return bare(s);
  }

  /* what: under the short title, one description line, only when the
     heading had to shorten the title: the whole title (a title that
     only adds "in <namespace>" counts as the same). The plain sentence
     of what is wrong is the "What's wrong?" question of the Summary
     tab; when it only says what the title already says, the question
     says so rather than repeat it. It also carries the objects a
     removed controller left behind. */
  function what(root, inc) {
    var short = core(KW.issueHeading(inc), inc), full = heading(inc);
    var w = sum(inc).what && KW.problemTitle(sum(inc).what, inc);
    var lead = KW.$("[data-ds-lead]", root), cut = core(full, inc) !== short;
    lead.textContent = "";
    lead.hidden = !cut;
    if (cut) KW.hideMarks(lead, full);
    var said = !core(w, inc) || core(w, inc) === short || (cut && core(w, inc) === core(full, inc));
    var dd = KW.summaryRow(root, "what", T.ask.what, true);
    if (!said) KW.hideMarks(dd.appendChild(KW.el("p", "ds-what")), w);
    else if (!(inc.leftoverObjects > 0)) dd.appendChild(KW.el("p", "ds-sub", T.ask.asTitle));
    KW.leftoverLine(dd, root, inc);
  }

  /* KW.summaryTop draws the summary card. */
  function summaryTop(root, inc) {
    header(root, inc);
    KW.issueStatus(root, inc);
    what(root, inc);
    KW.summaryRows(root, inc);
    if (KW.roundBlock) KW.roundBlock(root, inc);
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
    KW.fixSteps(root, inc);
    KW.logs(root, inc);
    KW.timeline(KW.$("[data-timeline]", root), KW.$("[data-tl-more]", root), inc);
    KW.facts(KW.$("[data-facts]", root), inc);
    KW.evidence(root, inc);
    KW.objects(root, inc);
    KW.leftovers(root, inc);
    KW.context(root, inc);
    if (KW.roundMore) KW.roundMore(root, inc);
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
        if (KW.incNav) KW.incNav.place(root);
        KW.title(heading(inc));
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
