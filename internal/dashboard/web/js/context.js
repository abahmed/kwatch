/* kwatch dashboard: an issue's context, in the tabs of the issue page.
   "What changed just before", "Seen before", "Who was told", "All
   steps to fix" and the similar issues. Every part hides when kwatch
   has nothing for it. API text is set with textContent only. */
(function () {
  "use strict";
  var KW = window.KW;

  var FIX = {
    rollback: "fixed by a rollback",
    "config fix": "fixed by a config change",
    "node replaced": "the node was replaced",
    change: "fixed by a change",
    "recovered without change": "recovered on its own"
  };

  /* KW.extraFold: a section the template has no place for, made once
     and put after the section named after. It returns the section and
     its body; the caller fills the body and sets hidden. */
  KW.extraFold = function (root, name, title, after) {
    var fold = KW.$('[data-fold="' + name + '"]', root);
    if (!fold) {
      fold = KW.issueSection(name, title);
      var prev = KW.$('[data-fold="' + after + '"]', root);
      prev.parentNode.insertBefore(fold, prev.nextSibling);
    }
    return { fold: fold, body: KW.$(".fold-body", fold) };
  };

  function stepNode(s) {
    var div = KW.el("div", "step");
    if (s.text) div.appendChild(KW.el("p", "", s.text));
    if (s.command) div.appendChild(KW.cmd(s.command));
    return div;
  }

  /* KW.runbook: "Your runbook for this", a link only for http(s). */
  KW.runbook = function (p, link) {
    p.textContent = "";
    p.hidden = !link;
    if (!link) return;
    var scheme = link.slice(0, link.indexOf(":") + 1).toLowerCase();
    if (scheme !== "https:" && scheme !== "http:") {
      p.textContent = "Your runbook for this: " + link;
      return;
    }
    var a = p.appendChild(KW.el("a", "ulink", "Your runbook for this"));
    a.href = link;
    a.target = "_blank";
    a.rel = "noopener noreferrer";
  };

  /* KW.fixSteps fills "All steps to fix" and the runbook. */
  KW.fixSteps = function (root, inc) {
    var list = inc.fixSteps || [], box = KW.$("[data-steps]", root);
    box.textContent = "";
    list.forEach(function (s, i) {
      var n = stepNode(s);
      n.insertBefore(KW.el("span", "step-n", "Step " + (i + 1)), n.firstChild);
      box.appendChild(n);
    });
    KW.runbook(KW.$("[data-runbook]", root), inc.docsLink);
    KW.$("[data-fix-n]", root).textContent = list.length ? "(" + list.length + ")" : "";
    var cut = KW.cutCount(inc, "fixSteps");
    if (cut) box.appendChild(KW.el("p", "ev-cap", KW.plural(cut, "more step", "more steps") + " not shown."));
    KW.$('[data-fold="fix"]', root).hidden = !list.length && !inc.docsLink;
  };

  /* KW.similar: "14 other issues have the same error → see them", a
     link to the table filtered to them (like=). */
  KW.similar = function (root, inc) {
    var p = KW.$("[data-similar]", root), n = inc.similar || 0;
    KW.$('[data-fold="similar"]', root).hidden = p.hidden = !n;
    if (!n) return;
    var a = KW.$("a", p);
    a.setAttribute("href", "/issues?status=all&like=" + encodeURIComponent(inc.id));
    a.textContent = n === 1 ? "1 other issue has the same error → see it" :
      KW.num(n) + " other issues have the same error → see them";
    a.title = "Issues that failed the same way, with the same likely cause, in any namespace. Active and resolved.";
  };

  /* ago: how long before or after the issue started, in words. */
  function offset(inc, iso) {
    var start = Date.parse(inc.firstSeen), at = Date.parse(iso);
    if (Math.abs(start - at) < 60000) return "when it started";
    if (at < start) return KW.lasted(iso, inc.firstSeen) + " before it started";
    return KW.lasted(inc.firstSeen, iso) + " after it started";
  }

  function objName(c) { return KW.obj(c.kind, c.namespace, c.name); }

  /* who: "by kubectl-edit · argocd/shop", or "". */
  function who(c) {
    var parts = [];
    if (c.actor) parts.push("by " + c.actor);
    if (c.app) parts.push("app " + c.app);
    return parts.join(" · ");
  }

  function changeRow(inc, c) {
    var li = KW.el("li", "cx-row");
    var when = KW.el("span", "cx-when cx-" + c.when, c.when === "before" ? "Before" : "During");
    when.title = c.when === "before" ? "Made before it started failing." :
      "Made while it was failing: maybe someone trying to fix it.";
    li.appendChild(when);
    var body = KW.el("div", "cx-body");
    var line = KW.el("p", "cx-line");
    line.appendChild(KW.el("span", "mono", objName(c)));
    line.appendChild(document.createTextNode(": " + c.summary));
    if (c.fixing) line.appendChild(KW.chip("c-resolved", "Likely fixed it"));
    if (c === blamed(inc)) {
      line.appendChild(KW.chip("c-notify", "Likely cause", "The change the likely cause points to."));
    }
    if (KW.isNew(c.at)) {
      li.className += " is-new";
      line.appendChild(KW.newMark());
    }
    body.appendChild(line);
    var meta = KW.el("small", "cx-meta");
    var t = KW.el("time", "", offset(inc, c.at));
    t.dateTime = c.at;
    t.title = KW.full(c.at);
    meta.appendChild(t);
    var w = who(c);
    if (w) meta.appendChild(document.createTextNode(" · " + w));
    if (c.note) meta.appendChild(document.createTextNode(" · recorded as “" + c.note + "”"));
    body.appendChild(meta);
    if ((c.fields || []).length) body.appendChild(diff(c));
    li.appendChild(body);
    return li;
  }

  /* hidden: whether a field is Secret or ConfigMap data, whose values
     kwatch never shows (the API sends "added", "changed", "removed" or
     nothing in their place). */
  function hidden(c, f) {
    var data = /^(data|binaryData|stringData)\./.test(f.path) ||
      /^(secret|configmap)$/i.test(c.kind || "");
    var marks = !f.before && !f.after ||
      /^(added|changed|removed)$/.test(f.after || "");
    return data && marks;
  }

  /* diff: the changed fields, before → after, folded under "Show what
     changed". Never "(none) → (none)": an empty side says "not set",
     and Secret or ConfigMap data says only that its value changed. */
  function diff(c) {
    var fields = c.fields;
    var det = KW.el("details", "cx-diff");
    det.dataset.keep = "";
    det.appendChild(KW.el("summary", "", "Show what changed (" +
      KW.plural(fields.length, "field", "fields") + ")"));
    var dl = det.appendChild(KW.el("dl", "cx-fields"));
    fields.forEach(function (f) {
      dl.appendChild(KW.el("dt", "mono", f.path));
      var dd = dl.appendChild(KW.el("dd"));
      if (hidden(c, f)) {
        var what = /^(added|removed)$/.test(f.after || "") ? f.after : "changed";
        dd.appendChild(KW.el("span", "cx-key", "Value " + what +
          " (kwatch never shows ConfigMap or Secret values)"));
        return;
      }
      if (!f.before && !f.after) {
        dd.appendChild(KW.el("span", "cx-key", "Changed (no value to show)"));
        return;
      }
      dd.appendChild(f.before ? KW.el("del", "mono cx-old", f.before) : KW.el("span", "cx-key", "not set"));
      dd.appendChild(document.createTextNode(" → "));
      dd.appendChild(f.after ? KW.el("ins", "mono cx-new", f.after) : KW.el("span", "cx-key", "not set"));
    });
    return det;
  }

  /* fixedLine: "Likely fixed by …" when kwatch judged a fix. */
  function fixedLine(inc) {
    var c = inc.fixedBy;
    if (c) return "Likely fixed by: " + objName(c) + ", " + c.summary + (who(c) ? " (" + who(c) + ")" : "");
    if (inc.state === "resolved" && FIX[inc.fix] && inc.fix !== "recovered without change") {
      return "How it ended: " + FIX[inc.fix] + ".";
    }
    return "";
  }

  function changes(root, inc) {
    var fold = KW.$('[data-fold="changes"]', root);
    var box = KW.$("[data-changes]", root);
    var list = (inc.changes || []).slice().sort(function (a, b) {
      return Date.parse(b.at) - Date.parse(a.at);
    });
    var fixed = fixedLine(inc);
    box.textContent = "";
    fold.hidden = !list.length && !fixed;
    if (fold.hidden) return;
    KW.$("[data-changes-n]", root).textContent = list.length ? "(" + list.length + ")" : "";
    if (fixed) box.appendChild(KW.el("p", "notice n-success cx-fixed", fixed));
    var ul = KW.el("ul", "cx-list");
    list.forEach(function (c) { ul.appendChild(changeRow(inc, c)); });
    if (list.length) box.appendChild(ul);
    var cut = KW.cutCount(inc, "changes");
    box.appendChild(KW.el("p", "ev-cap", "Changes people made to this object and what it uses, " +
      "from 30 minutes before it started" + (inc.state === "resolved" ? " until it was resolved." : " until now.") +
      (cut ? " " + KW.plural(cut, "more change", "more changes") + " not shown." : "")));
  }

  /* recovered: "Changed just before it recovered", on a resolved issue.
     It only says what changed; it never says that was the fix. */
  function recovered(root, inc) {
    var x = KW.extraFold(root, "recovered", "Changed just before it recovered", "changes");
    var list = inc.state === "resolved" ? inc.changedBeforeRecovery || [] : [];
    x.fold.hidden = !list.length;
    x.body.textContent = "";
    if (!list.length) return;
    x.body.appendChild(KW.el("p", "ev-cap", "Made shortly before it stopped failing. " +
      "This may not be the reason it recovered."));
    var ul = x.body.appendChild(KW.el("ul", "cx-list"));
    list.forEach(function (c) {
      var li = ul.appendChild(KW.el("li", "cx-row cx-plain"));
      var line = li.appendChild(KW.el("p", "cx-line"));
      line.appendChild(KW.el("span", "mono", objName(c)));
      line.appendChild(document.createTextNode(": " + c.summary));
      var meta = li.appendChild(KW.el("small", "cx-meta"));
      meta.appendChild(KW.hm(c.at));
      if (who(c)) meta.appendChild(document.createTextNode(" · " + who(c)));
    });
    var cut = KW.cutCount(inc, "changedBeforeRecovery");
    if (cut) x.body.appendChild(KW.el("p", "ev-cap", KW.plural(cut, "more change", "more changes") + " not shown."));
  }

  function fixWords(fix) { return FIX[fix] || ""; }

  function seen(root, inc) {
    var fold = KW.$('[data-fold="seen"]', root);
    var box = KW.$("[data-seen]", root);
    var s = inc.seenBefore;
    box.textContent = "";
    fold.hidden = !s || !s.recent || !s.recent.length;
    if (fold.hidden) return;
    var n = s.inWindow;
    box.appendChild(KW.el("p", "", n ?
      "It failed the same way " + KW.plural(n, "time", "times") + " in the last " + s.windowDays + " days before this." :
      "It failed the same way before, but not in the last " + s.windowDays + " days."));
    var ul = KW.el("ul", "seen-list");
    s.recent.forEach(function (o) {
      var li = KW.el("li");
      li.appendChild(KW.el("span", "mono", KW.stamp(o.opened)));
      var parts = [];
      if (KW.valid(o.resolved)) parts.push("lasted " + KW.lasted(o.opened, o.resolved));
      if (fixWords(o.fix)) parts.push(fixWords(o.fix));
      if (parts.length) li.appendChild(document.createTextNode(" · " + parts.join(" · ")));
      li.title = KW.full(o.opened);
      ul.appendChild(li);
    });
    box.appendChild(ul);
    var cap = s.capped ? "kwatch keeps only the last " + s.recent.length + " times, so there may be more." : "";
    var cut = KW.cutCount(inc, "occurrences");
    if (cut) cap += (cap ? " " : "") + KW.plural(cut, "earlier time", "earlier times") + " not shown.";
    if (cap) box.appendChild(KW.el("p", "ev-cap", cap));
    if (s.previousId) {
      var a = KW.el("a", "ulink", "Open the last time it happened");
      a.href = "/issues/" + encodeURIComponent(s.previousId);
      a.dataset.link = "";
      box.appendChild(KW.el("p", "seen-prev")).appendChild(a);
    }
  }

  /* blamed: the change the likely cause names: the one the API marks,
     else the one the checks agree on (the correlation's "Change"),
     else one the cause text names other than the issue's own object. */
  function blamed(inc) {
    var list = inc.changes || [];
    var hit = list.filter(function (c) { return c.cause; })[0];
    if (hit) return hit;
    var fact = ((inc.correlation || {}).facts || []).filter(function (f) {
      return f.label === "Change";
    })[0];
    if (fact) {
      hit = list.filter(function (c) {
        return fact.value.indexOf(c.kind + " " + c.name + " ") === 0;
      })[0];
      if (hit) return hit;
    }
    var cause = inc.rootCause || "";
    return list.filter(function (c) {
      return c.when === "before" && c.name !== inc.workload && cause.indexOf(c.name) >= 0;
    })[0] || null;
  }

  /* CAPTIONS: one muted line under each section heading, so the
     explanation is not only a tooltip. */
  var CAPTIONS = {
    why: "What kwatch found, and how sure it is.",
    others: "Issues that look linked to this one.",
    fix: "Every step, in order.",
    changes: "Changes near the time it started, nearest first.",
    seen: "Earlier times this object failed the same way.",
    waiting: "kwatch is checking before it alerts.",
    probes: "The health check that is failing.",
    warnings: "Recent warnings from the cluster.",
    traffic: "How much traffic this affects.",
    node: "The machine this runs on.",
    timeline: "Newest first.",
    logs: "The latest lines from the container.",
    containers: "What runs inside the pod.",
    related: "Objects hit by the same problem.",
    told: "Which channels kwatch told, and who acknowledged.",
    more: "Technical details.",
    blast: "What this failure reaches beyond the failing object.",
    similar: "Other issues with the same error.",
    sent: "The text kwatch sent to each channel."
  };

  /* captions adds the lines once; a section's own body is rebuilt
     by its module, so the caption sits beside the body, not in it. */
  function captions(root) {
    KW.$$("section.sec[data-fold]", root).forEach(function (d) {
      var text = CAPTIONS[d.dataset.fold];
      if (!text || d.querySelector(":scope > .fold-cap")) return;
      var body = d.querySelector(":scope > .fold-body");
      if (body) d.insertBefore(KW.el("p", "fold-cap", text), body);
    });
    KW.$$(".fold-h[title]", root).forEach(function (h) {
      if (h.nextElementSibling && h.nextElementSibling.classList.contains("fold-cap")) return;
      h.parentNode.insertBefore(KW.el("p", "fold-cap", h.title), h.nextSibling);
    });
  }

  KW.context = function (root, inc) {
    changes(root, inc);
    recovered(root, inc);
    seen(root, inc);
    KW.told(root, inc);
    KW.sent(root, inc);
    // What the cluster says now, and sharing (insight.js, share.js).
    if (KW.insight) KW.insight(root, inc);
    if (KW.share) KW.share(root, inc);
    captions(root);
  };
})();
