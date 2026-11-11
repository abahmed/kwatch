/* kwatch dashboard: the questions at the top of the issue page: the
   status line under the title, Why? (the cause in a plain sentence,
   the release it followed, the decisive proof line and its key
   number), What should I do? (the release's undo first, then the
   steps, one Copy each) and Who was alerted?. Each fact is said once;
   the rest is a tab away. API text is set with textContent only. */
(function () {
  "use strict";
  var KW = window.KW, T = KW.TERMS;
  var SHOWN_ACTIONS = 3;

  function sum(inc) { return inc.summary || {}; }
  function ms(iso) { return Date.parse(iso); }
  function bare(x) { return String(x || "").toLowerCase().replace(/[^a-z0-9]+/g, " ").trim(); }

  /* ---- images and releases, in short words ---- */

  function shortImage(ref) { return String(ref || "").split("/").pop(); }
  function tagOf(ref) { var s = shortImage(ref), i = s.lastIndexOf(":"); return i < 0 ? "" : s.slice(i + 1); }
  function repoOf(ref) { var s = shortImage(ref), i = s.lastIndexOf(":"); return i < 0 ? s : s.slice(0, i); }

  /* imageText: "2.83.0-beta.4 → 2.83.0-beta.5": tags when the repository
     is the same, else the two short names; the registry is never shown. */
  function imageText(im) {
    var a = tagOf(im.before), b = tagOf(im.after);
    if (!a || !b || repoOf(im.before) !== repoOf(im.after)) { a = shortImage(im.before); b = shortImage(im.after); }
    return im.before ? a + " → " + b : "added " + shortImage(im.after);
  }

  function actor(r) {
    var by = r.app || r.actor || "";
    if (/^argocd/i.test(by) || /argocd/i.test(r.actor || "")) return "Argo CD";
    if (/^helm/i.test(by)) return "Helm";
    if (/^flux/i.test(by) || /flux/i.test(r.actor || "")) return "Flux";
    return by;
  }

  var BAD = { failed: "rollout failed", down: "no replica ready", regressed: "restarts far more" };

  /* releaseLine: "Release 10:34 · image 2.83.0-beta.4 → beta.5 · by
     Argo CD", the full image names in its tooltip. */
  function releaseLine(r) {
    var p = KW.el("p", "ds-rel"), bits = [];
    var full = (r.images || []).map(function (im) { return (im.before ? im.before + " → " : "") + im.after; });
    p.appendChild(document.createTextNode("Release "));
    p.appendChild(KW.hm(r.at));
    if ((r.images || []).length) bits.push("image " + imageText(r.images[0]) +
      (r.images.length > 1 ? " and " + (r.images.length - 1) + " more" : ""));
    if (r.changed && r.changed.length && !(r.images || []).length) bits.push("changed " + r.changed.join(", "));
    if (actor(r)) bits.push("by " + actor(r));
    if (BAD[r.verdict]) bits.push(BAD[r.verdict]);
    bits.forEach(function (b) { p.appendChild(document.createTextNode(" · " + b)); });
    var d = r.detail || {};
    if (typeof d.restartsAfter === "number" && d.restartsAfter > 0) {
      var rate = function (n) { return (Math.round(n * 10) / 10).toLocaleString("en-US"); };
      p.appendChild(document.createTextNode(" · restarts per hour: " + rate(d.restartsAfter) +
        (typeof d.restartsBefore === "number" ? " (usually " + rate(d.restartsBefore) + ")" : "")));
    }
    p.title = full.join("\n") + (r.app ? "\n" + r.app : "") + (r.actor ? "\n" + r.actor : "");
    return p;
  }

  /* ---- the cause in plain words ---- */

  var ENGINE = /^(\S+) (\S+?)(?: \([^)]*\))? explains (\d+) failures?$/;

  /* sinceRelease: "It started 1 minute after the release (image a → b)". */
  function sinceRelease(inc) {
    var r = inc.release;
    if (!r || !KW.valid(r.at) || !KW.valid(inc.firstSeen)) return "";
    var gap = ms(inc.firstSeen) - ms(r.at), s = "It started ";
    if (gap < 0) s += "as the release rolled out";
    else if (gap < 60000) s += "less than a minute after the release";
    else s += KW.lasted(r.at, inc.firstSeen).replace(/^1 min$/, "1 minute").replace(/ min$/, " minutes") + " after the release";
    var im = (r.images || [])[0];
    return s + (im ? " (image " + imageText(im) + ")" : "") + ".";
  }

  /* causeText: the sentence for Why?. kwatch's engine wording ("deployment
     x explains 4 failures", from an older server) is never shown. */
  function causeText(inc, text) {
    var m = ENGINE.exec(text || "");
    if (!m) return text;
    return sinceRelease(inc) || ("The " + m[1].toLowerCase() + " " + m[2] +
      (+m[3] > 1 ? " is the common cause of " + m[3] + " failing objects." : " is the cause."));
  }

  /* ---- the decisive proof line (log lines: the Go picker's twin) ---- */

  var ERR = /\b(error|err|panic|fatal|exception|fail(ed|ure)?|refused|denied|timeout|timed out|cannot|can't|unable|traceback|killed|unhealthy|not ready)\b|\w(error|exception)\b/i;
  var SCAFFOLD = /^(at\s|---(\s|$)|-{4,}|Traceback \(|File "|goroutine \d|\.{3}\s*\d+ (more|common)|\[?stack ?trace)/i;
  var CAUSE = /^(--->\s*|Caused by:\s*)/;

  /* KW.bestLogLine: the log line that states the error: the innermost
     cause of a wrapped exception, never the stack trace around it. */
  KW.bestLogLine = function (lines) {
    var plain = "", cause = "";
    (lines || []).forEach(function (l) {
      l = String(l).trim();
      if (!l || !ERR.test(l) || SCAFFOLD.test(l)) return;
      if (CAUSE.test(l)) cause = l.replace(CAUSE, "");
      else plain = l;
    });
    return cause || plain;
  };

  function scaffold(line) { return SCAFFOLD.test(String(line).trim()); }

  var LEAD = { log: 0, probe: 1, scheduler: 2, event: 3, node: 4, change: 5, exit: 6, endpoints: 7 };

  /* decisive: the proof that says what fails, with its line. */
  function decisive(inc) {
    var list = (sum(inc).proof || []).filter(function (p) { return p.kind !== "endpoints" && p.kind !== "exit"; });
    list.sort(function (a, b) { return (!!b.detail - !!a.detail) || (LEAD[a.kind] - LEAD[b.kind]); });
    var p = list[0];
    if (p && p.kind === "log" && scaffold(p.detail)) {
      var best = KW.bestLogLine(inc.capturedLogLines);
      p = best ? { kind: "log", text: p.text, detail: best } : null;
    }
    if (!p && inc.release && inc.release.detail && inc.release.detail.errorLine) {
      p = { kind: "log", text: "First error of the new version", detail: inc.release.detail.errorLine };
    }
    return p || null;
  }

  /* keyNumber: "It has restarted 95 times", from the exit proof. */
  function keyNumber(inc) {
    var ex = (sum(inc).proof || []).filter(function (p) { return p.kind === "exit" && p.count > 1; })[0];
    if (ex) return "It has restarted " + KW.plural(ex.count, "time", "times") + (ex.detail ? ". Last exit: " + ex.detail + "." : ".");
    var pr = (sum(inc).proof || []).filter(function (p) { return p.kind === "probe" && p.count > 1; })[0];
    return pr ? pr.text + ": failed " + KW.plural(pr.count, "time", "times") + "." : "";
  }

  /* raw: the exact line in monospace with its own Copy. */
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
    box.appendChild(KW.copyButton(text, "the exact text"));
    return box;
  }

  var CONF = { likely: "kwatch thinks this is the likely cause.", possible: "This is a possible cause, not a certain one." };

  function whyRow(root, inc) {
    var w = sum(inc).why || {}, text = w.text || inc.rootCause || "";
    var known = w.causeKnown === undefined ? !!text : !!w.causeKnown && !!text;
    var dd = KW.summaryRow(root, "why", T.ask.why, true);
    if (known) {
      KW.hideMarks(dd.appendChild(KW.el("p", "ds-why-t")), causeText(inc, text));
      if (CONF[w.confidence]) dd.appendChild(KW.el("p", "ds-sub", CONF[w.confidence]));
    } else {
      unknownCause(dd, inc);
    }
    if (inc.release) dd.appendChild(releaseLine(inc.release));
    proofBlock(dd, root, inc);
  }

  function unknownCause(dd, inc) {
    var viaRelease = KW.releaseWhy ? KW.releaseWhy(inc) : "";
    if (viaRelease) { dd.appendChild(KW.el("p", "ds-why-t", viaRelease)); return; }
    // No single cause, but the investigation narrowed it to two.
    var ck = inc.round && inc.round.check;
    if (ck && ck.cause && ck.rival) {
      dd.appendChild(KW.el("p", "ds-why-t", "One of two causes: " + KW.entityText(ck.cause) + " or " + KW.entityText(ck.rival) + "."));
      dd.appendChild(KW.el("p", "ds-sub", "kwatch ran a check to tell them apart; see what it found below."));
      return;
    }
    dd.appendChild(KW.el("p", "ds-why-t ds-unknown", inc.state === "resolved" ? "Cause not found." : "Cause not found yet."));
    var more = inc.state === "settling" ? "kwatch is still collecting what failed around it." : "";
    if (inc.evidence && inc.evidence.length) more = [more, "Open Proof to see what kwatch checked."].filter(Boolean).join(" ");
    if (KW.checkedText) more = [more, KW.checkedText(inc)].filter(Boolean).join(" ");
    if (more) dd.appendChild(KW.el("p", "ds-sub", more));
  }

  /* proofBlock: the decisive line with Copy, the key number and the
     link to every other proof. */
  function proofBlock(dd, root, inc) {
    var p = decisive(inc), n = keyNumber(inc);
    var total = (sum(inc).proof || []).length + KW.cutCount(inc, "summaryProof");
    if (p) {
      var cap = dd.appendChild(KW.el("p", "ds-cap"));
      cap.appendChild(KW.el("b", "", T.proof + ": "));
      KW.hideMarks(cap.appendChild(KW.el("span")), p.text);
      if (p.detail) dd.appendChild(raw(p.detail));
    }
    if (n) dd.appendChild(KW.el("p", "ds-key", n));
    if (total > (p ? 1 : 0) + (n ? 1 : 0) || (inc.evidence && inc.evidence.length)) {
      var b = dd.appendChild(KW.el("button", "link ds-steps-b", "See all " + T.proof.toLowerCase() + " →"));
      b.type = "button";
      b.addEventListener("click", function () { KW.showTab(root, "evidence"); });
    }
  }

  /* ---- what should I do ---- */

  /* actions: the release's undo first, then the steps, up to
     SHOWN_ACTIONS; a command is listed once. used counts the steps
     read, so the rest are the ones left for the Details tab. */
  function actions(inc) {
    var out = [], seen = {}, r = inc.release, used = 0;
    function add(text, command) {
      if (command) { if (seen[command]) return; seen[command] = true; }
      out.push({ text: text, command: command });
    }
    if (r && r.undo && (r.undo.text || r.undo.command) && BAD[r.verdict]) {
      add(/[.!?]$/.test(r.undo.text || "") ? r.undo.text : (r.undo.text || "Undo the release") + ".", r.undo.command);
    }
    (inc.fixSteps || []).forEach(function (s) {
      if (out.length >= SHOWN_ACTIONS) return;
      used += 1;
      add(s.text, s.command);
    });
    return { list: out, used: used };
  }

  var LOGS = /^(deployment|statefulset|daemonset|replicaset|job|pod)$/;

  /* firstLook: without fix steps, the read-only kubectl commands that
     show what Kubernetes reports about the object, and the proof. */
  function firstLook(dd, root, inc) {
    var kind = String(inc.workloadKind || "").toLowerCase(), name = inc.workload;
    var ns = inc.namespace ? "-n " + inc.namespace + " " : "", obj = kind + "/" + name;
    var list = [];
    if (kind && name) list.push(["Its state and recent events:", "kubectl " + ns + "describe " + obj]);
    if (kind && name && LOGS.test(kind)) {
      list.push(["Its latest logs:", "kubectl " + ns + "logs " + obj + " --all-containers --tail=50"]);
    }
    list.push(["Recent warnings around it:", "kubectl " + (ns || "-A ") +
      "get events --field-selector type=Warning --sort-by=.lastTimestamp"]);
    dd.appendChild(KW.el("p", "ds-sub", T.ask.noSteps));
    var ol = dd.appendChild(KW.el("ol", "ds-todo"));
    list.forEach(function (x) {
      var li = ol.appendChild(KW.el("li"));
      li.appendChild(KW.el("p", "ds-next-t", x[0]));
      li.appendChild(KW.cmd(x[1]));
    });
    var b = dd.appendChild(KW.el("button", "link ds-steps-b", "See what kwatch found in " + T.proof + " →"));
    b.type = "button";
    b.addEventListener("click", function () { KW.showTab(root, "evidence"); });
  }

  function todoRow(root, inc) {
    var a = actions(inc), steps = inc.fixSteps || [];
    var dd = KW.summaryRow(root, "next", T.ask.todo, true);
    root.dataset.stepsShown = String(a.used);
    if (!a.list.length) firstLook(dd, root, inc);
    if (a.list.length) {
      var ol = dd.appendChild(KW.el("ol", "ds-todo"));
      a.list.forEach(function (x) {
        var li = ol.appendChild(KW.el("li"));
        if (x.text) KW.hideMarks(li.appendChild(KW.el("p", "ds-next-t")), x.text);
        if (x.command) li.appendChild(KW.cmd(x.command));
      });
    }
    var rest = steps.length - a.used + KW.cutCount(inc, "summaryNext");
    if (rest > 0) {
      var b = dd.appendChild(KW.el("button", "link ds-steps-b", KW.plural(rest, "more step", "more steps") + " →"));
      b.type = "button";
      b.addEventListener("click", function () { KW.openFold(root, "fix"); });
    }
    if (inc.docsLink) KW.runbook(dd.appendChild(KW.el("p", "ds-runbook")), inc.docsLink);
  }

  /* ---- who was alerted ---- */

  var PAGER = { open: "alerted · still open", closed: "alerted · closed", failed: "alert failed", pending: "alert pending, will retry" };
  var SENT = { failed: "failed", pending: "sending, will retry" };

  function first(inc) {
    var t = inc.timing || {};
    if (KW.valid(t.toldAt)) return { name: t.toldVia || "a channel", at: t.toldAt };
    var ok = (inc.deliveries || []).filter(function (d) { return d.result === "ok" && d.name && KW.valid(d.at); })[0];
    return ok ? { name: ok.name, at: ok.at } : null;
  }

  /* KW.firstAlert: who was alerted first and when, or null. */
  KW.firstAlert = first;
  /* KW.proofLine: the real error line of the issue, for a ticket. */
  KW.proofLine = function (inc) {
    var p = decisive(inc);
    return p && p.detail ? String(p.detail) : "";
  };

  function line(dd, label, body) {
    var p = dd.appendChild(KW.el("p", "ds-alert"));
    p.appendChild(KW.el("b", "", label + " "));
    body(p);
    return p;
  }

  function toldRow(root, inc) {
    var dd = KW.summaryRow(root, "told", T.ask.told, true), list = sum(inc).told || [];
    var f = first(inc), t = inc.timing || {};
    if (!f && !list.length) {
      dd.appendChild(KW.el("p", "ds-sub", KW.nobodyText(inc)));
      ackLine(dd, inc);
      return;
    }
    if (f) {
      line(dd, "First alert:", function (p) {
        p.appendChild(document.createTextNode(f.name + " · "));
        p.appendChild(KW.hm(f.at));
        var from = KW.valid(t.startedAt) ? t.startedAt : inc.firstSeen;
        var gap = ms(f.at) - ms(from);
        if (KW.valid(from) && gap >= 60000) p.appendChild(document.createTextNode(" · " + KW.lasted(from, f.at) + " after it started"));
      });
    }
    var latest = list.filter(function (c) { return c.kind !== "pager" && KW.valid(c.at); })
      .sort(function (a, b) { return ms(b.at) - ms(a.at); })[0];
    if (latest && (!f || ms(latest.at) - ms(f.at) >= 60000)) {
      line(dd, "Last update:", function (p) {
        p.appendChild(document.createTextNode(latest.name || latest.provider));
        p.appendChild(document.createTextNode(" · "));
        p.appendChild(KW.hm(latest.at));
      });
    }
    list.forEach(function (c) {
      var word = c.kind === "pager" ? PAGER[c.state] : SENT[c.state];
      if (!word) return;
      var p = dd.appendChild(KW.el("p", "ds-alert" + (c.state === "failed" ? " t-bad" : "")));
      p.textContent = (c.name || c.provider) + ": " + word;
    });
    ackLine(dd, inc);
    var all = dd.appendChild(KW.el("button", "link ds-steps-b", "All alerts →"));
    all.type = "button";
    all.addEventListener("click", function () { KW.openFold(root, "told"); });
  }

  function ackLine(dd, inc) {
    if (inc.state === "resolved") return;
    var a = inc.ack, p = dd.appendChild(KW.el("p", "ds-alert"));
    if (a || inc.ackedBy) {
      p.appendChild(KW.el("b", "", "Acknowledged"));
      p.appendChild(document.createTextNode(a && a.by ? " by " + a.by : ""));
      if (a && KW.valid(a.since)) { p.appendChild(document.createTextNode(" · ")); p.appendChild(KW.hm(a.since)); }
    } else {
      p.appendChild(KW.el("b", "", "Not acknowledged yet."));
      // The same command the Alerts tab shows: to copy, never a button
      // that acts (this page only reads the cluster).
      var cmd = KW.ackCommand ? KW.ackCommand(inc) : "";
      if (cmd) {
        p.appendChild(document.createTextNode(" To acknowledge it, run:"));
        dd.appendChild(KW.cmd(cmd));
      }
    }
  }

  /* ---- the status line ---- */

  /* impact: what the data says about users: the old version still
     serving, or the endpoints that are ready. */
  function impact(inc) {
    var r = inc.release, rows = inc.traffic || [];
    if ((r && r.oldServing) || /previous version is still serving/i.test(inc.title || "")) {
      return "users are still served by the old version";
    }
    var lost = rows.filter(function (x) { return x.total > 0 && x.ready < x.total; })
      .sort(function (a, b) { return (b.total - b.ready) - (a.total - a.ready); })[0];
    if (!lost) return "";
    if (lost.ready === 0) return "no " + lost.kind + " endpoint is ready, so " + lost.name + " is unreachable";
    return lost.ready + " of " + lost.total + " endpoints of " + lost.kind + " " + lost.name + " are ready";
  }

  /* KW.issueStateChip: the state as a chip: "Happening now",
     "Resolved", "Stopped tracking" or "Settled before alerting". */
  KW.issueStateChip = function (inc) {
    var st = KW.STATE[inc.state] || {}, resolved = inc.state === "resolved";
    var text = resolved ? (inc.stoppedTracking ? "Stopped tracking" :
      KW.isUntold(inc) ? T.settled.label : "Resolved") : st.label || inc.state || "";
    var c = KW.chip((st.chip || "c-state") + " ds-state s-" + inc.state, text, st.title || "");
    c.dataset.dsStatus = "";
    return c;
  };

  /* KW.issueStatus: what the state means beyond its chip: "users are
     still served by the old version", "it keeps failing and
     recovering". The header lists them after where and since when. */
  KW.issueStatus = function (root, inc) {
    var bits = [];
    if (inc.state !== "resolved" && impact(inc)) bits.push(impact(inc));
    if (inc.state === "flapping" || (sum(inc).stage || {}).flapping) {
      bits.push("it keeps failing and recovering");
    }
    return bits;
  };

  KW.summaryRows = function (root, inc) {
    whyRow(root, inc);
    todoRow(root, inc);
    toldRow(root, inc);
  };
  KW.causeSentence = function (inc) {
    var w = sum(inc).why || {};
    return causeText(inc, w.text || inc.rootCause || "");
  };
})();
