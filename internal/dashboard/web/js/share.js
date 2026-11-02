/* kwatch dashboard: sharing an issue from its summary card. "Copy
   summary" copies a short text for chat or a ticket, built here from
   the issue's data. */
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

  KW.share = function (root, inc) {
    var b = KW.$("[data-copy-summary]", root);
    b.onclick = function () {
      var link = location.origin + "/issues/" + encodeURIComponent(inc.id);
      KW.copy(b, KW.summaryText(inc, link), "Copy summary");
    };
  };
})();
