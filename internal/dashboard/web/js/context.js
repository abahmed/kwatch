/* kwatch dashboard: an issue's context. "What changed just before",
   "Seen before" and "Who was told", and their one-line hints in the
   "What's happening" card. Every part hides when kwatch has nothing
   for it. API text is set with textContent only. */
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

  /* RESULT: what became of a message, as the word after it ("update
     sent"), its class and tooltip. */
  var RESULT = {
    ok: ["sent", "", "The channel accepted the message."],
    failed: ["failed", "x-bad", "The channel refused it. kwatch tried its fallback, if one is set."],
    retrying: ["will be sent again", "x-warn", "The channel did not answer. kwatch sends it again later."],
    carried: ["listed in a summary", "", "Not sent on its own: a summary message listed it."],
    "not routed": ["not sent", "", "No channel is set up to receive this kind of alert."],
    "paging only": ["not sent", "", "Meant only for paging tools, and none is set up."]
  };

  var WHAT = {
    root: "first alert", thread: "update", edit: "resolved (edited the alert)",
    standalone: "resolved message"
  };

  /* openFold opens a folded section of the issue and scrolls to it. */
  KW.openFold = function (root, name) {
    var fold = KW.$('[data-fold="' + name + '"]', root);
    if (!fold || fold.hidden) return;
    fold.open = true;
    fold.scrollIntoView({ block: "start" });
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

  /* toldRow: "18:51  Slack · first alert sent". */
  function toldRow(d) {
    var li = KW.el("li", "told-row");
    li.appendChild(KW.hm(d.at)).className = "mono";
    var body = KW.el("div", "told-body");
    var head = KW.el("p");
    var r = RESULT[d.result] || [d.result, "", ""];
    head.appendChild(KW.el("b", "", d.name || (d.carrier ? "The " + d.carrier : "No channel")));
    head.appendChild(document.createTextNode(" · " + (d.kind === "resolve" ? "resolved message" :
      d.kind === "summary" ? "summary" : WHAT[d.placement] || "message") + " "));
    var res = head.appendChild(KW.el("span", r[1], r[0]));
    if (r[2]) res.title = r[2];
    body.appendChild(head);
    var meta = [];
    if (d.retries) meta.push("after " + KW.plural(d.retries, "retry", "retries"));
    if (d.fallbackFor) meta.push("fallback for " + d.fallbackFor);
    if (meta.length) body.appendChild(KW.el("small", "cx-meta", meta.join(" · ")));
    if (d.error) body.appendChild(KW.el("p", "ch-msg", d.error));
    li.appendChild(body);
    return li;
  }

  /* told fills "Who was told". Whether someone acknowledged it is said
     once, in "What's going on with this alert" (incwhy.js). */
  function told(root, inc) {
    var fold = KW.$('[data-fold="told"]', root);
    var box = KW.$("[data-told]", root);
    var list = inc.deliveries || [];
    box.textContent = "";
    fold.hidden = !list.length;
    if (fold.hidden) return;
    var ul = KW.el("ul", "told-list");
    list.slice().reverse().forEach(function (d) { ul.appendChild(toldRow(d)); });
    box.appendChild(ul);
    var cut = KW.cutCount(inc, "deliveries");
    box.appendChild(KW.el("p", "ev-cap", "What kwatch remembers since it last started." +
      (cut ? " " + KW.plural(cut, "older message", "older messages") + " not shown." : "")));
  }

  /* code: the HTTP status in a channel's error ("401"), or "". */
  function code(err) {
    var m = /\b([45]\d\d)\b/.exec(err || "");
    return m ? m[1] : "";
  }

  /* toldText: who was told, failures first, each channel with the
     time it first took a message: "PagerDuty failed (401) · fell back
     to Opsgenie 19:06 · Slack 18:51 · not acknowledged". */
  function toldText(inc) {
    var list = inc.deliveries || [], by = {}, order = [];
    list.forEach(function (d) {
      var name = d.name || "";
      if (!name) return;
      if (!by[name]) { by[name] = {}; order.push(name); }
      var c = by[name];
      if (d.result === "ok" && !c.ok) c.ok = d;
      if (d.result === "failed") c.failed = d;
      if (d.result === "retrying") c.retrying = d;
    });
    var bad = [], fell = [], fine = [];
    order.forEach(function (name) {
      var c = by[name];
      if (c.ok) {
        var at = name + " " + KW.when(c.ok.at);
        if (c.ok.fallbackFor) fell.push("fell back to " + at);
        else fine.push(at);
      } else if (c.failed) {
        var n = code(c.failed.error);
        bad.push(name + " failed" + (n ? " (" + n + ")" : ""));
      } else if (c.retrying) {
        bad.push("still trying " + name);
      }
    });
    // Nothing went wrong: "Sent to Slack 18:51 · …".
    if (!bad.length && !fell.length && fine.length) fine[0] = "sent to " + fine[0];
    var parts = bad.concat(fell, fine);
    if (!parts.length) {
      var d = list.filter(function (x) { return x.result === "carried"; })[0];
      if (!list.length) return "";
      parts.push(d ? "Listed in the " + (d.carrier || "summary") + " " + KW.when(d.at) :
        "Not sent to any channel");
    }
    parts.push(inc.ackedBy ? "acknowledged" : "not acknowledged");
    var text = parts.join(" · ");
    return text.charAt(0).toUpperCase() + text.slice(1);
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

  /* changedText: the change the cause names, else the nearest one
     before it started, for the summary card. */
  function changedText(inc) {
    var list = inc.changes || [];
    var c = blamed(inc) || list.filter(function (x) { return x.when === "before"; })[0] || list[0];
    if (!c) return "";
    var start = Date.parse(inc.firstSeen), at = Date.parse(c.at);
    var lead = Math.abs(start - at) < 60000 ? "Changed when it started" :
      at < start ? "Changed " + KW.lasted(c.at, inc.firstSeen) + " before" :
      "Changed " + KW.lasted(inc.firstSeen, c.at) + " after it started";
    return lead + ": " + objName(c) + ", " + c.summary;
  }

  function seenText(inc) {
    var s = inc.seenBefore;
    if (!s || !s.inWindow) return "";
    return "Seen " + KW.plural(s.inWindow, "time", "times") + " before in " + s.windowDays + " days";
  }

  /* hints fills the summary card's lines; each opens its section. */
  function hints(root, inc) {
    var ul = KW.$("[data-ctx]", root);
    ul.textContent = "";
    [[changedText(inc), "changes", "What changed around it, nearest first."],
      [seenText(inc), "seen", "Earlier times the same object failed the same way."],
      [toldText(inc), "told", "Which alert channels kwatch told, and whether someone acknowledged it."]
    ].forEach(function (h) {
      if (!h[0]) return;
      var b = KW.el("button", "link d-ctx-b", h[0]);
      b.type = "button";
      b.title = h[2];
      b.onclick = function () { KW.openFold(root, h[1]); };
      ul.appendChild(KW.el("li")).appendChild(b);
    });
    ul.hidden = !ul.firstChild;
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
    timeline: "Each change, oldest to newest.",
    logs: "The latest lines from the container.",
    containers: "What runs inside the pod.",
    related: "Objects hit by the same problem.",
    told: "Which channels kwatch told, and who acknowledged.",
    more: "Technical details."
  };

  /* captions adds the lines once; a section's own body is rebuilt
     by its module, so the caption sits beside the body, not in it. */
  function captions(root) {
    KW.$$("details.fold[data-fold]", root).forEach(function (d) {
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
    captions(root);
    changes(root, inc);
    seen(root, inc);
    told(root, inc);
    hints(root, inc);
    // What the cluster says now, and sharing (insight.js, share.js).
    if (KW.insight) KW.insight(root, inc);
    if (KW.share) KW.share(root, inc);
  };
})();
