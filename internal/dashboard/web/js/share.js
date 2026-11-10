/* kwatch dashboard: alerting people about an issue. "Copy summary" in
   the summary card copies a short text for chat or a ticket. In the
   Alerts tab, "Who was alerted" lists the first alert, the updates
   (repeats folded) and the resolve, and "What we sent" the text kwatch
   sent. All built from the issue's data. */
(function () {
  "use strict";
  var KW = window.KW;

  function pad(n) { return n < 10 ? "0" + n : "" + n; }

  /* utc: "2026-10-01 12:00 UTC", the same for everyone who reads it. */
  function utc(iso) {
    var d = new Date(iso);
    return d.getUTCFullYear() + "-" + pad(d.getUTCMonth() + 1) + "-" + pad(d.getUTCDate()) +
      " " + pad(d.getUTCHours()) + ":" + pad(d.getUTCMinutes()) + " UTC";
  }

  /* code wraps a command so chat apps show it as code. */
  function code(command) {
    if (command.indexOf("`") < 0 && command.indexOf("\n") < 0) return "`" + command + "`";
    return "```\n" + command + "\n```";
  }

  function where(inc) {
    var obj = (inc.workloadKind ? inc.workloadKind + " " : "") + (inc.workload || "whole cluster");
    return inc.namespace ? inc.namespace + " · " + obj : obj;
  }

  /* summaryText is the issue in a few lines of plain text, with
     Markdown bold and code that Slack and most trackers understand. */
  KW.summaryText = function (inc, link) {
    var sev = KW.SEVERITY[inc.severity];
    var lines = ["*" + (inc.fullTitle || inc.title) + "*"];
    lines.push("Severity: " + (sev ? sev.label + " (" + sev.level.toLowerCase() + ")" : inc.severity));
    if (inc.state === "resolved" && KW.valid(inc.resolvedAt)) {
      lines.push("Resolved: " + utc(inc.resolvedAt) + " (lasted " +
        KW.lasted(inc.firstSeen, inc.resolvedAt) + ")");
    } else {
      var st = KW.STATE[inc.state];
      if (st && inc.state !== "open") lines.push("State: " + st.label);
    }
    lines.push("Where: " + where(inc));
    if (KW.valid(inc.firstSeen)) lines.push("Since: " + utc(inc.firstSeen));
    if (inc.rootCause) lines.push("Likely cause: " + (KW.causeSentence ? KW.causeSentence(inc) : inc.rootCause));
    var cluster = KW.cluster && typeof KW.cluster === "object" ? KW.cluster.name : KW.cluster;
    if (cluster && typeof cluster === "string") lines.push("Cluster: " + cluster);
    var proof = KW.proofLine ? KW.proofLine(inc) : "";
    if (proof) lines.push("Proof: " + (proof.length > 300 ? proof.slice(0, 299) + "…" : proof));
    var sent = KW.firstAlert ? KW.firstAlert(inc) : null;
    lines.push("Alerted: " + (sent ? sent.name + " at " + utc(sent.at) :
      "nobody" + (KW.isUntold(inc) ? " (settled on its own)" : "")));
    var step = (inc.fixSteps || [])[0];
    if (step) {
      if (step.text) lines.push("First step: " + step.text);
      if (step.command) lines.push(code(step.command));
    }
    lines.push("Details: " + link);
    return lines.join("\n");
  };

  /* RESULT: what became of a message, as the word after it ("update
     sent"), its class and tooltip. */
  var RESULT = {
    ok: ["sent", "", "The channel accepted the message."],
    failed: ["failed", "x-bad", "The channel refused it. kwatch tried its fallback, if one is set."],
    retrying: ["will be sent again", "x-warn", "The channel did not answer. kwatch sends it again later."],
    carried: ["bundled in the summary message", "", "Not sent on its own: the summary message listed it."],
    "not routed": ["not sent", "", "No channel is set up to receive this kind of alert."],
    "paging only": ["not sent", "", "Meant only for paging tools, and none is set up."]
  };

  var WHAT = {
    root: "first alert", thread: "update", edit: "resolved (edited the alert)",
    standalone: "resolved message"
  };

  function kindWord(d) {
    return d.kind === "resolve" ? "resolved message" : d.kind === "summary" ? "summary" : WHAT[d.placement] || "message";
  }

  function who(d) {
    return d.name || (d.carrier ? "Summary message" : "No channel");
  }

  /* groups folds messages that are the same thing to the same place,
     one after another, into one entry with every time. */
  function groups(list) {
    var out = [];
    list.forEach(function (d) {
      var key = d.result === "carried" ? "carried" : [who(d), kindWord(d), d.result, d.reason || ""].join("|"), last = out[out.length - 1];
      if (last && last.key === key) last.all.push(d);
      else out.push({ key: key, d: d, all: [d] });
    });
    return out;
  }

  function times(body, list, label) {
    if (!label) return;
    var det = body.appendChild(KW.el("details", "tl-times"));
    det.appendChild(KW.el("summary", "", label));
    det.appendChild(KW.el("p", "mono", list.map(function (x) { return KW.stamp(x.at); }).join(" · ")));
  }

  /* toldRow: "18:51  Slack · first alert sent", with "3 more updates". */
  function toldRow(g) {
    var d = g.d, li = KW.el("li", "told-row");
    li.appendChild(KW.hm(d.at)).className = "mono";
    var body = KW.el("div", "told-body");
    var head = KW.el("p");
    var r = RESULT[d.result] || [d.result, "", ""];
    if (d.result === "carried") {
      // Not sent on its own: said once, never as "update bundled in ...".
      var bundled = head.appendChild(KW.el("b", "", "Bundled in the summary message"));
      bundled.title = r[2];
      head.appendChild(document.createTextNode(" · " + (g.all.length > 1 ? KW.plural(g.all.length, "message", "messages") : kindWord(d))));
      body.appendChild(head);
      times(body, g.all, g.all.length > 1 ? "Show every time" : "");
      li.appendChild(body);
      return li;
    }
    head.appendChild(KW.el("b", "", who(d)));
    head.appendChild(document.createTextNode(" · " + kindWord(d) + " "));
    var res = head.appendChild(KW.el("span", r[1], r[0]));
    if (r[2]) res.title = r[2];
    // Why a send failed, in plain words, next to the failure.
    if (d.reason && (d.result === "failed" || d.result === "retrying")) {
      head.appendChild(document.createTextNode(" · " + d.reason));
    }
    body.appendChild(head);
    var meta = [];
    if (d.retries) meta.push("after " + KW.plural(d.retries, "retry", "retries"));
    if (d.fallbackFor) meta.push("fallback for " + d.fallbackFor);
    if (meta.length) body.appendChild(KW.el("small", "cx-meta", meta.join(" · ")));
    if (d.error) body.appendChild(KW.el("p", "ch-msg", d.error));
    if (g.all.length > 1) {
      var more = g.all.length - 1, det = body.appendChild(KW.el("details", "tl-times"));
      det.appendChild(KW.el("summary", "", KW.plural(more, "more " + kindWord(d), "more " + kindWord(d) + "s")));
      det.appendChild(KW.el("p", "mono", g.all.slice(1).map(function (x) { return KW.stamp(x.at); }).join(" · ")));
    }
    li.appendChild(body);
    return li;
  }

  /* KW.told fills "Who was alerted" in the Alerts tab, oldest first.
     Whether someone acknowledged it is in "Alert status" (incwhy.js). */
  KW.told = function (root, inc) {
    var fold = KW.$('[data-fold="told"]', root);
    var box = KW.$("[data-told]", root);
    var list = inc.deliveries || [];
    box.textContent = "";
    fold.hidden = !list.length;
    KW.$("h3", fold).textContent = "Who was alerted";
    if (fold.hidden) return;
    var ul = KW.el("ul", "told-list");
    groups(list).forEach(function (g) { ul.appendChild(toldRow(g)); });
    box.appendChild(ul);
    var cut = KW.cutCount(inc, "deliveries");
    box.appendChild(KW.el("p", "ev-cap", "What kwatch remembers since it last started." +
      (cut ? " " + KW.plural(cut, "older message", "older messages") + " not shown." : "")));
  };

  /* KW.sent fills "What we sent": the exact text kwatch composed for
     each channel, folded away, with its time. It is plain text before
     each app's own formatting. */
  KW.sent = function (root, inc) {
    var x = KW.extraFold(root, "sent", "What we sent", "told");
    var list = inc.sent || [];
    x.fold.hidden = !list.length;
    x.body.textContent = "";
    if (!list.length) return;
    list.forEach(function (m) {
      var head = x.body.appendChild(KW.el("p", "sent-h"));
      head.appendChild(KW.el("b", "", m.name || m.provider));
      if (KW.valid(m.at)) {
        head.appendChild(document.createTextNode(" · "));
        head.appendChild(KW.hm(m.at));
      }
      if (m.cut) head.appendChild(KW.el("span", "muted", " (shortened)"));
      var pre = x.body.appendChild(KW.el("pre", "sent-text"));
      KW.hideMarks(pre, m.text || "");
    });
    var cut = KW.cutCount(inc, "sent");
    x.body.appendChild(KW.el("p", "ev-cap", "Plain text before each app's own formatting." +
      (cut ? " " + KW.plural(cut, "more message", "more messages") + " not shown." : "")));
  };

  KW.share = function (root, inc) {
    var b = KW.$("[data-copy-summary]", root);
    b.onclick = function () {
      var link = location.origin + "/issues/" + encodeURIComponent(inc.id);
      KW.copy(b, KW.summaryText(inc, link), "Copy summary");
    };
  };
})();
