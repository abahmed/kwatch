/* kwatch dashboard: an issue's objects. "Containers": the table (as
   kwatch sees them now, or as they were when it broke), then memory and
   CPU now, then the usage chart (usagechart.js). "Affected objects"
   with ready commands. Bars are sized with CSSOM, never with inline
   style text. */
(function () {
  "use strict";
  var KW = window.KW;

  var EXIT = {
    OOMKilled: "It used more memory than its limit, so Kubernetes stopped it.",
    Error: "The program exited with an error.",
    Completed: "The program finished and exited.",
    ContainerCannotRun: "Kubernetes could not start the program.",
    StartError: "Kubernetes could not start the program."
  };

  var STATUS = {
    failing: ["Failing", "s-failing", "This object is failing now."],
    cause: ["Likely cause", "s-cause", "kwatch thinks the failure starts here."],
    affected: ["Affected", "s-affected", "Hit by the failure, but not failing itself."],
    recovered: ["Recovered", "s-recovered", "The issue is resolved."],
    gone: ["Deleted", "s-gone", "This object was deleted while the issue was open."]
  };

  /* cell adds a table cell; label shows on narrow screens. */
  function cell(tr, label, content, cls) {
    var td = KW.el("td", cls || "");
    td.dataset.label = label;
    if (content && content.nodeType) td.appendChild(content);
    else td.textContent = content === undefined || content === null ? "" : content;
    tr.appendChild(td);
    return td;
  }

  function grid(heads, cls) {
    var t = KW.el("table", "kt " + (cls || ""));
    var tr = t.appendChild(KW.el("thead")).appendChild(KW.el("tr"));
    heads.forEach(function (h) {
      var th = KW.el("th", "", h[0]);
      th.scope = "col";
      if (h[1]) th.title = h[1];
      tr.appendChild(th);
    });
    t.appendChild(KW.el("tbody"));
    return t;
  }

  function tagOf(image) {
    return (image || "").split("/").pop();
  }

  /* healthy: ready, never restarted and not stopped by an error. */
  function healthy(c) {
    var bad = c.lastReason && c.lastReason !== "Completed" || (c.lastExitCode && c.lastExitCode !== 0);
    return c.ready !== false && !c.restarts && !bad;
  }

  function lastExit(c) {
    if (!c.lastReason && c.lastExitCode === undefined) return KW.el("span", "muted", "None");
    var text = (c.lastReason || "Exited") + (c.lastExitCode !== undefined ? " (" + c.lastExitCode + ")" : "");
    var s = KW.el("span", c.lastReason === "OOMKilled" || c.lastExitCode ? "x-bad" : "", text);
    s.title = EXIT[c.lastReason] || (c.lastExitCode === 137 ?
      "Stopped by a kill signal, often for using too much memory." : "How the last run ended.");
    return s;
  }

  function readyText(c) {
    if (c.ready === undefined) return c.state || "Unknown";
    var text = c.ready ? "Yes" : "No";
    if (!c.ready && c.stateReason) text += " · " + c.stateReason;
    return text;
  }

  /* restarts: the count and a bar against the most restarted
     container shown (KW.viz.meter). */
  function restarts(n, top) {
    var box = KW.el("span", "rs");
    box.appendChild(KW.el("span", "rs-n", KW.num(n)));
    if (top && KW.viz) {
      box.appendChild(KW.viz.meter(n, top, KW.plural(n, "restart", "restarts") + " (most here: " + KW.num(top) + ")", "warn"));
    }
    return box;
  }

  /* views: the container sets to choose from: the snapshots kwatch
     kept when it broke and got worse, then the live rows. */
  function views(inc) {
    var out = (inc.snapshots || (inc.snapshot ? [inc.snapshot] : [])).filter(function (s) {
      return s && KW.valid(s.at) && (s.containers || []).length;
    }).map(function (s) {
      var worse = s.reason === "got worse";
      return { key: "s" + Date.parse(s.at), rows: s.containers, at: s.at,
        label: (worse ? "When it got worse" : "At first break") + " (" + KW.when(s.at) + ")",
        cap: (worse ? "As they were when it got worse, " : "As they were when it first broke, ") + KW.stamp(s.at) + "." };
    });
    if ((inc.containers || []).length) {
      out.push({ key: "now", rows: inc.containers, label: "Now",
        cap: "As kwatch sees them now, most restarted first." });
    }
    return out;
  }

  function switcher(box, list, pick, redraw) {
    var bar = KW.el("div", "tz snap-sw");
    bar.setAttribute("role", "group");
    bar.setAttribute("aria-label", "Show the containers");
    list.forEach(function (v) {
      var b = bar.appendChild(KW.el("button", "tz-b", v.label));
      b.type = "button";
      b.setAttribute("aria-pressed", v.key === pick.key ? "true" : "false");
      b.addEventListener("click", function () {
        box.dataset.view = v.key;
        redraw();
      });
    });
    box.appendChild(bar);
  }

  /* unknown: an empty cell, "—", for a time kwatch does not know. */
  function unknown() {
    var s = KW.el("span", "muted", "—");
    s.title = "Not known";
    return s;
  }

  /* KW.containersFold shows "Containers" when any of its parts (the
     table, usage now, the usage chart) has something. */
  KW.containersFold = function (root) {
    KW.$('[data-fold="containers"]', root).hidden = !["[data-containers]",
      "[data-resources]", "[data-usage]"].some(function (s) {
      return KW.$(s, root).firstChild;
    });
  };

  function containers(root, inc) {
    var box = KW.$("[data-containers]", root);
    var list = views(inc);
    box.textContent = "";
    if (list.length) table(box, list, inc, function () { containers(root, inc); });
    var snap = KW.cutCount(inc, "snapshots"), rows = KW.cutCount(inc, "snapshotRows");
    if (snap) box.appendChild(KW.el("p", "ev-cap", KW.plural(snap, "more snapshot", "more snapshots") + " not shown."));
    if (rows) box.appendChild(KW.el("p", "ev-cap", KW.plural(rows, "more container row", "more container rows") + " not shown."));
    KW.containersFold(root);
  }

  /* table draws the containers of the view picked (box.dataset.view). */
  function table(box, list, inc, redraw) {
    var pick = list.filter(function (v) { return v.key === box.dataset.view; })[0] ||
      list[list.length - 1];
    if (list.length > 1) switcher(box, list, pick, redraw);
    var t = grid([["Container"], ["Ready", "Whether Kubernetes sends it traffic."],
      ["Restarts"], ["Last exit", "How its last run ended, and the exit code."],
      ["Last died"], ["Image", "The image tag it runs now."]], "kt-stack");
    var body = KW.$("tbody", t), top = 0;
    pick.rows.forEach(function (c) { top = Math.max(top, c.restarts || 0); });
    // Healthy containers (sidecars) wait behind a toggle while another fails.
    var sick = pick.rows.filter(function (c) { return !healthy(c); }).length;
    var all = box.dataset.healthy === "1" || !sick;
    pick.rows.filter(function (c) { return all || !healthy(c); }).forEach(function (c) {
      var tr = body.appendChild(KW.el("tr"));
      var name = KW.wrapName(c.pod + " / " + c.container);
      cell(tr, "Container", name, "kt-name");
      cell(tr, "Ready", readyText(c), c.ready === false ? "x-bad" : "");
      cell(tr, "Restarts", restarts(c.restarts || 0, top), c.restarts ? "x-num x-warn" : "x-num");
      cell(tr, "Last exit", lastExit(c));
      cell(tr, "Last died", KW.valid(c.lastFinished) ? KW.timeEl(c.lastFinished) : unknown());
      var img = KW.el("span", "mono", tagOf(c.image));
      img.title = c.image || "";
      cell(tr, "Image", img, "kt-img");
    });
    box.appendChild(KW.el("div", "kt-wrap")).appendChild(t);
    var well = pick.rows.length - sick;
    if (sick && well) {
      var tog = box.appendChild(KW.el("button", "btn btn-sm kt-more", all ? "Hide healthy containers" :
        "+" + KW.plural(well, "healthy container", "healthy containers")));
      tog.type = "button";
      tog.setAttribute("aria-expanded", all ? "true" : "false");
      tog.addEventListener("click", function () {
        box.dataset.healthy = all ? "" : "1";
        redraw();
      });
    }
    var cut = pick.key === "now" ? KW.cutCount(inc, "containers") : 0;
    box.appendChild(KW.el("p", "ev-cap", pick.cap +
      (cut ? " " + KW.plural(cut, "more container", "more containers") + " not shown." : "")));
  }

  /* bytes: "980 Mi", "1 Gi", "512 Ki". */
  function bytes(n) {
    var units = [["Gi", 1 << 30], ["Mi", 1 << 20], ["Ki", 1 << 10]];
    for (var i = 0; i < units.length; i++) {
      if (n >= units[i][1]) {
        var v = n / units[i][1];
        return (v >= 10 || v === Math.round(v) ? Math.round(v) : v.toFixed(1)) + " " + units[i][0];
      }
    }
    return n + " B";
  }

  function milli(n) { return n >= 1000 && n % 100 === 0 ? (n / 1000) + " CPU" : n + "m"; }
  KW.fmtBytes = bytes;
  KW.fmtMilli = milli;

  /* usage is one resource line with its bar, or null without a reading. */
  function usage(name, used, limit, request, fmt) {
    if (!used) return null;
    var of = limit || request;
    var row = KW.el("div", "use-row");
    var text = name + " " + fmt(used);
    var pct = 0;
    if (of) {
      pct = Math.round(used / of * 100);
      text += " of " + fmt(of) + " " + (limit ? "limit" : "request") + " (" + pct + "%)";
    } else {
      text += " (no limit set)";
    }
    row.appendChild(KW.el("p", "use-text", text));
    if (of) {
      var bar = KW.el("div", "use-bar");
      bar.setAttribute("role", "img");
      bar.setAttribute("aria-label", text);
      var fill = bar.appendChild(KW.el("span", pct >= 90 ? "use-fill hot" : pct >= 75 ? "use-fill warm" : "use-fill"));
      fill.style.width = Math.min(100, pct) + "%";
      row.appendChild(bar);
    }
    return row;
  }

  function resources(root, inc) {
    var box = KW.$("[data-resources]", root);
    var list = (inc.containers || []).filter(function (c) { return c.memoryUsed || c.cpuUsedMilli; });
    box.textContent = "";
    if (list.length) usageNow(box, list);
    KW.containersFold(root);
  }

  /* usageNow: memory and CPU now, against the limit (or request). */
  function usageNow(box, list) {
    box.appendChild(KW.el("h3", "fold-h", "Memory and CPU now"));
    var latest = "";
    list.forEach(function (c) {
      var item = KW.el("div", "use");
      item.appendChild(KW.el("p", "use-name mono", c.pod + " / " + c.container));
      [usage("Memory", c.memoryUsed, c.memoryLimit, c.memoryRequest, bytes),
        usage("CPU", c.cpuUsedMilli, c.cpuLimitMilli, c.cpuRequestMilli, milli)
      ].forEach(function (r) { if (r) item.appendChild(r); });
      box.appendChild(item);
      if (KW.valid(c.usageAt) && (!latest || Date.parse(c.usageAt) > Date.parse(latest))) latest = c.usageAt;
    });
    var cap = KW.el("p", "ev-cap", "Read from the node");
    if (latest) {
      cap.appendChild(document.createTextNode(" "));
      cap.appendChild(KW.timeEl(latest));
    }
    cap.appendChild(document.createTextNode(". Memory is what the container uses now (its working set)."));
    box.appendChild(cap);
  }

  function copyButton(label, command) {
    var b = KW.el("button", "btn btn-sm", label);
    b.type = "button";
    b.title = command;
    b.addEventListener("click", function () { KW.copy(b, command, label); });
    return b;
  }

  /* commands: a row's log commands; the describe commands are one Copy
     above the table. */
  function commands(o) {
    var box = KW.el("div", "ao-cmds");
    (o.commands || []).slice(1).forEach(function (c) {
      var m = / -c (\S+) --previous$/.exec(c);
      box.appendChild(copyButton("Copy logs" + (m ? " of " + m[1] : ""), c));
    });
    return box;
  }

  function affected(root, inc) {
    var fold = KW.$('[data-fold="related"]', root);
    var box = KW.$("[data-affected]", root);
    var list = inc.affected || [];
    box.textContent = "";
    if (!list.length) return;
    fold.hidden = false;
    var describes = list.map(function (o) { return (o.commands || [])[0]; }).filter(Boolean);
    if (describes.length) {
      var bar = box.appendChild(KW.el("p", "ao-bar"));
      bar.appendChild(copyButton(describes.length === 1 ? "Copy describe command" :
        "Copy describe commands for all " + describes.length, describes.join("\n")));
      bar.appendChild(KW.el("span", "muted small", " Paste in a terminal that can reach your cluster."));
    }
    var logs = list.some(function (o) { return (o.commands || []).length > 1; });
    var heads = [["Status"], ["Kind"], ["Name"], ["Started failing"]];
    if (logs) heads.push(["Logs"]);
    var t = grid(heads, "kt-stack");
    var body = KW.$("tbody", t);
    list.forEach(function (o) {
      var tr = body.appendChild(KW.el("tr"));
      var s = STATUS[o.status] || [o.status, "s-affected", ""];
      var st = KW.el("span", "ao-status");
      st.appendChild(KW.el("span", "ao-dot " + s[1]));
      st.appendChild(document.createTextNode(s[0]));
      st.title = s[2];
      cell(tr, "Status", st);
      var kind = KW.el("span", "", o.kind);
      if (o.root) {
        kind.appendChild(KW.el("small", "ao-main", "this issue's object"));
        kind.title = "The object this issue is about.";
      }
      cell(tr, "Kind", kind);
      cell(tr, "Name", KW.wrapName((o.namespace ? o.namespace + "/" : "") + o.name), "kt-name");
      cell(tr, "Started failing", KW.valid(o.since) ? KW.timeEl(o.since) : KW.el("span", "muted", "—"));
      if (logs) cell(tr, "Logs", commands(o));
    });
    box.appendChild(KW.el("div", "kt-wrap")).appendChild(t);
    var cut = KW.cutCount(inc, "affected");
    if (cut) box.appendChild(KW.el("p", "ev-cap", "+" + KW.plural(cut, "more object", "more objects") + " not shown."));
  }

  KW.objects = function (root, inc) {
    containers(root, inc);
    resources(root, inc);
    affected(root, inc);
    KW.issueMap(root, inc);
    KW.strip(root, inc);
  };
  /* KW.liveObjects redraws only what changes between reads. */
  KW.liveObjects = function (root, inc) {
    containers(root, inc);
    resources(root, inc);
  };
})();
