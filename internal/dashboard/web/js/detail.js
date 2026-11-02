/* kwatch dashboard: Issue detail, in the order of a kwatch alert. */
(function () {
  "use strict";
  var KW = window.KW;

  var BANNERS = {
    flapping: ["n-degraded", "Keeps coming back",
      "It keeps failing and recovering. kwatch stays quiet unless it changes."],
    recovering: ["n-resolved", "Looks better",
      "It looks fixed. kwatch waits a little to be sure."]
  };

  /* stopped: the sentence for an issue kwatch stopped tracking: it is
     not fixed. A workload also says what was still wrong. */
  function stopped(inc) {
    var text = "kwatch stopped tracking this after " +
      KW.lasted(inc.firstSeen, inc.resolvedAt) + "; it is still not healthy, so check it yourself.";
    if (inc.workloadKind && inc.workload) {
      text += inc.stillCrashing ? " A container still keeps crashing." :
        " It is still short of replicas.";
    }
    return text;
  }

  function stepNode(s) {
    var div = KW.el("div", "step");
    if (s.text) div.appendChild(KW.el("p", "", s.text));
    if (s.command) div.appendChild(KW.cmd(s.command));
    return div;
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

  /* fix: the first step goes in the summary ("What to do first"); the
     full list and the runbook wait in "All steps to fix". */
  function fix(root, inc) {
    var list = inc.fixSteps || [];
    var first = KW.$("[data-first]", root);
    var firstBox = KW.$("[data-first-step]", root);
    firstBox.textContent = "";
    if (list.length) firstBox.appendChild(stepNode(list[0]));
    else if (inc.docsLink) runbook(firstBox.appendChild(KW.el("p")), inc.docsLink);
    first.hidden = !list.length && !inc.docsLink;
    var box = KW.$("[data-steps]", root);
    box.textContent = "";
    list.forEach(function (s) { box.appendChild(stepNode(s)); });
    runbook(KW.$("[data-runbook]", root), inc.docsLink);
    KW.$("[data-fix-n]", root).textContent = "(" + list.length + ")";
    var cut = KW.cutCount(inc, "fixSteps");
    if (cut) box.appendChild(KW.el("p", "ev-cap", KW.plural(cut, "more step", "more steps") + " not shown."));
    KW.$('[data-fold="fix"]', root).hidden = list.length < 2 && !cut &&
      !(list.length && inc.docsLink);
  }

  /* shortNote: one line at the end of the page when the server
     shortened any long text to keep the response small. */
  function shortNote(root, inc) {
    var p = KW.$("[data-short]", root);
    if (!p) {
      p = KW.el("p", "ev-cap d-short", "Some long text was shortened.");
      p.dataset.short = "";
      KW.$(".detail", root).appendChild(p);
    }
    p.hidden = !KW.cutCount(inc, "texts");
  }

  /* oneLine: "1 Nov, 18:50" (with UTC when the page shows UTC) on one
     line; the exact time in both zones is in its tooltip. */
  function oneLine(iso) {
    var t = KW.el("time", "d-nowrap", KW.stamp(iso) + (KW.utc ? " UTC" : ""));
    t.setAttribute("datetime", iso);
    t.title = KW.full(iso);
    return t;
  }

  /* facts: the exact times and IDs, under "More details". */
  function facts(dl, inc) {
    dl.textContent = "";
    function add(label, value, cls, tip) {
      var dt = dl.appendChild(KW.el("dt", "", label));
      if (tip) dt.title = tip;
      var dd = KW.el("dd", cls || "");
      if (typeof value === "string") dd.textContent = value;
      else dd.appendChild(value);
      dl.appendChild(dd);
    }
    var sev = KW.SEVERITY[inc.severity];
    add("Alert level", sev ? sev.level : inc.severity, "",
      "How kwatch told your team about this.");
    add("Started", oneLine(inc.firstSeen));
    add("Last seen", KW.timeEl(inc.lastSeen));
    if (inc.repeatCount > 0) add("Failed again", KW.plural(inc.repeatCount, "time", "times"));
    if (inc.state === "resolved" && KW.valid(inc.resolvedAt)) {
      add(inc.stoppedTracking ? "Stopped tracking" : "Resolved", KW.stamp(inc.resolvedAt) + " (lasted " +
        KW.lasted(inc.firstSeen, inc.resolvedAt) + ")");
    }
    if (inc.ackedBy) {
      add("Acknowledged", "Yes, on " + inc.ackedBy, "mono",
        "Someone marked this as seen with kwatch's annotation on that " +
        "object, so kwatch stops reminding.");
    }
    add("Issue ID", inc.id, "mono");
  }

  /* summary fills "What's happening": badges, title, where, since. */
  function summary(root, inc) {
    var resolved = inc.state === "resolved";
    var chips = KW.$("[data-chips]", root);
    chips.textContent = "";
    if (resolved) {
      chips.appendChild(inc.stoppedTracking ?
        KW.chip("c-notify", "Stopped tracking", "kwatch gave up following this; it was not fixed.") :
        KW.chip("c-resolved", "Resolved"));
      chips.appendChild(KW.severityChip(inc.severity, "Was "));
    } else {
      chips.appendChild(KW.severityChip(inc.severity));
      chips.appendChild(KW.stateChip(inc.state));
    }
    if (inc.ackedBy) {
      chips.appendChild(KW.chip("c-acked", "Acknowledged",
        "Someone marked this as seen, so kwatch stops reminding."));
    }
    KW.$("[data-title]", root).textContent = inc.title;
    // The likely cause, right under the title: the one-line answer.
    var cause = inc.rootCause || "";
    KW.$("[data-likely]", root).hidden = !cause;
    KW.$("[data-likely-text]", root).textContent = cause;
    var where = KW.$("[data-where]", root);
    where.textContent = "";
    if (inc.namespace) {
      where.appendChild(KW.el("span", "mono", inc.namespace));
      where.appendChild(document.createTextNode(" · "));
    }
    if (inc.workloadKind) where.appendChild(KW.el("span", "", inc.workloadKind + " "));
    where.appendChild(KW.el("span", "mono", inc.workload || "whole cluster"));
    var since = KW.$("[data-since]", root);
    since.textContent = "";
    var lbl = KW.$("[data-since-label]", root);
    if (resolved && KW.valid(inc.resolvedAt)) {
      lbl.textContent = "Lasted";
      since.textContent = KW.lasted(inc.firstSeen, inc.resolvedAt) +
        (inc.stoppedTracking ? " · stopped tracking " : " · resolved ");
      since.appendChild(KW.timeEl(inc.resolvedAt));
      return;
    }
    lbl.textContent = "Since";
    since.appendChild(KW.timeEl(inc.firstSeen));
    if (inc.repeatCount > 0) {
      since.appendChild(document.createTextNode(" · " +
        KW.failedAgain(inc.repeatCount).toLowerCase()));
    }
  }

  /* EXPLAINS: the section that is the explanation for a kind of issue,
     opened by itself on the first draw: the failing probe, why it is
     waiting to start, or the memory chart (in "Containers"). */
  var EXPLAINS = [
    ["probes", function (inc) { return (inc.probes || []).length > 0; }],
    ["waiting", function (inc) { return !!inc.waiting; }],
    ["containers", function (inc) {
      return (inc.usageHours || []).some(function (h) {
        return h.metric !== "cpu" && (h.peaks || []).length;
      });
    }]
  ];

  /* autoOpen opens that section once per issue. It is marked data-auto
     so incpanel.js still redraws it; a click on it makes it the
     reader's. */
  function autoOpen(root, inc) {
    if (root.dataset.autoFor === inc.id) return;
    root.dataset.autoFor = inc.id;
    var hit = EXPLAINS.filter(function (e) { return e[1](inc); })[0];
    var fold = hit && KW.$('[data-fold="' + hit[0] + '"]', root);
    if (!fold || fold.hidden || fold.open) return;
    fold.open = true;
    fold.dataset.auto = "";
    KW.$("summary", fold).addEventListener("click", function () {
      fold.removeAttribute("data-auto");
    }, { once: true });
  }

  /* similar: "14 other issues have the same error → see them", a link
     to the table filtered to them (like=). */
  function similar(root, inc) {
    var p = KW.$("[data-similar]", root), n = inc.similar || 0;
    p.hidden = !n;
    if (!n) return;
    var a = KW.$("a", p);
    a.setAttribute("href", "/issues?status=all&like=" + encodeURIComponent(inc.id));
    a.textContent = n === 1 ? "1 other issue has the same error → see it" :
      n.toLocaleString("en-US") + " other issues have the same error → see them";
    a.title = "Issues that failed the same way, with the same likely cause, " +
      "in any namespace. Active and resolved.";
  }

  /* KW.renderIncident fills a tpl-detail clone (root) with an issue.
     The issue page and the preview drawer of the Issues table both
     draw with it. */
  KW.renderIncident = function (root, inc) {
    // First: what is new since the last visit marks the parts below.
    KW.lastLook(root, inc);
    var resolved = inc.state === "resolved";
    var banner = KW.$("[data-banner]", root);
    var b = BANNERS[inc.state];
    if (resolved && KW.valid(inc.resolvedAt)) {
      b = inc.stoppedTracking ? ["n-degraded", "Stopped tracking", stopped(inc)] :
        ["n-success", "Resolved", "It stopped failing " + KW.stamp(inc.resolvedAt) + "."];
    }
    // Recovering but unseen: the alert box says so; "looks fixed" would not.
    if (b && !resolved && inc.why && inc.why.cannotSee && inc.state === "recovering") {
      b = [b[0], b[1], "kwatch can't see this right now, so it stays open until it can check again."];
    }
    banner.hidden = !b;
    if (b) {
      banner.className = "notice d-banner " + b[0];
      KW.$("b", banner).textContent = b[1];
      KW.$("p", banner).textContent = b[2];
    }
    summary(root, inc);
    similar(root, inc);
    KW.alertBox(root, inc);
    // A link to this issue, to paste into chat or a ticket.
    var copyLink = KW.$("[data-copy-link]", root);
    copyLink.onclick = function () {
      KW.copy(copyLink, location.origin + "/issues/" + encodeURIComponent(inc.id), "Copy link");
    };
    KW.$("[data-cause]", root).textContent = inc.rootCause || (inc.state === "settling" ?
      "Not sure yet. kwatch is still collecting what failed around it." :
      "kwatch has no single cause for this.");
    fix(root, inc);
    KW.logs(root, inc);
    KW.timeline(KW.$("[data-timeline]", root), KW.$("[data-tl-more]", root), inc);
    facts(KW.$("[data-facts]", root), inc);
    KW.evidence(root, inc);
    KW.objects(root, inc);
    KW.context(root, inc);
    KW.links(root, inc);
    KW.usageChart(root, inc);
    autoOpen(root, inc);
    shortNote(root, inc);
  };

  function notFound(main) {
    KW.title("Issue not found");
    main.textContent = "";
    main.appendChild(KW.el("h1", "", "We can't find that issue"));
    var p = KW.el("p", "lead-404", "It may have been resolved more than 7 days ago, or the link is wrong.");
    main.appendChild(p);
    var links = KW.el("p", "links-404");
    [["/issues", "Active issues"], ["/issues?status=resolved", "Resolved"]].forEach(function (l, i) {
      if (i) links.appendChild(document.createTextNode(" · "));
      var a = KW.el("a", "ulink", l[1]);
      a.href = l[0];
      a.dataset.link = "";
      links.appendChild(a);
    });
    main.appendChild(links);
  }

  /* KW.detailView is the issue page, /issues/<id>: the issue
     (incpanel.js), its tab title and the link back to the table. */
  KW.detailView = function (main, r) {
    var panel = KW.incidentPanel(main, r.id, {
      page: true,
      scrollBack: KW.takeScroll(),
      each: KW.refreshBadge,
      notFound: function () { notFound(main); },
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
