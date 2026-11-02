/* kwatch dashboard: telling people about an issue. "Copy summary" in
   the summary card copies a short text for chat or a ticket; "Who was
   told" lists what each alert channel did, and "What we sent" the text
   kwatch sent, folded away. All built from the issue's data. */
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
    var lines = ["*" + inc.title + "*"];
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
    if (inc.rootCause) lines.push("Likely cause: " + inc.rootCause);
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
    carried: ["listed in a summary", "", "Not sent on its own: a summary message listed it."],
    "not routed": ["not sent", "", "No channel is set up to receive this kind of alert."],
    "paging only": ["not sent", "", "Meant only for paging tools, and none is set up."]
  };

  var WHAT = {
    root: "first alert", thread: "update", edit: "resolved (edited the alert)",
    standalone: "resolved message"
  };

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
    li.appendChild(body);
    return li;
  }

  /* KW.told fills "Who was told". Whether someone acknowledged it is
     said once, in "Alert status" (incwhy.js). */
  KW.told = function (root, inc) {
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
  };

  /* statusCode: the HTTP status in a channel's error ("401"), or "". */
  function statusCode(err) {
    var m = /\b([45]\d\d)\b/.exec(err || "");
    return m ? m[1] : "";
  }

  /* toldText: who was told, failures first, each channel with the
     time it first took a message: "PagerDuty failed (401) · fell back
     to Opsgenie 19:06 · Slack 18:51 · not acknowledged". */
  KW.toldText = function (inc) {
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
        var n = statusCode(c.failed.error);
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
