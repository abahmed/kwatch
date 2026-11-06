/* kwatch dashboard: the release linked to an issue (inc.release, from
   the API's release book). A row of the issue's summary, between the
   proof and what to do: the workload, when it rolled out, how it went,
   what changed and how to undo it. Shown only when a release is linked.
   The dashboard never changes the cluster: the undo is a command to
   copy. */
(function () {
  "use strict";
  var KW = window.KW;

  /* VERDICTS: the chip class and the sentence of each verdict. A bad
     verdict says "Likely caused by"; the others only place a release
     near the issue. */
  var VERDICTS = {
    failed: ["c-page", "Failed", "The new version fails while the old one still serves.", true],
    down: ["c-page", "Down", "No replica of the workload is ready.", true],
    regressed: ["c-page", "Regressed", "The new version restarts far more than the previous one.", true],
    watching: ["c-notify", "Watching", "Rolled out. kwatch is still watching it.", false],
    rolling: ["c-notify", "Rolling out", "New pods are still replacing the old ones.", false],
    healthy: ["c-resolved", "Healthy", "Nothing broke after it rolled out.", false],
    replaced: ["c-low", "Replaced", "A newer release came before kwatch reached a verdict.", false],
    unknown: ["c-quiet", "Unknown", "kwatch saw it after the fact, so it did not watch it roll out.", false]
  };

  var CHANGED = { environment: "environment", resources: "resources", probes: "probes",
    config: "config", volumes: "volumes", restart: "a restart", "pod template": "the pod template" };

  function mono(text) { return KW.el("span", "mono", text); }

  function who(r) {
    var kind = r.kind ? r.kind.charAt(0).toUpperCase() + r.kind.slice(1) : "";
    return [r.name, kind, r.namespace].filter(Boolean).join(" · ");
  }

  /* timing: "Rolled out 14:02, 38 min before this started." */
  function timing(r, inc) {
    var p = KW.el("p", "rl-when");
    p.appendChild(document.createTextNode("Rolled out "));
    p.appendChild(KW.hm(r.at));
    if (KW.valid(r.at) && KW.valid(inc.firstSeen)) {
      var gap = Date.parse(inc.firstSeen) - Date.parse(r.at);
      var span = KW.lasted(gap >= 0 ? r.at : inc.firstSeen, gap >= 0 ? inc.firstSeen : r.at);
      p.appendChild(document.createTextNode(", " + span + (gap >= 0 ? " before" : " after") +
        " this started."));
    } else {
      p.appendChild(document.createTextNode("."));
    }
    return p;
  }

  /* changes: the images and the other parts the release changed. */
  function changes(r) {
    var ul = KW.el("ul", "rl-list");
    (r.images || []).forEach(function (im) {
      var li = ul.appendChild(KW.el("li"));
      li.appendChild(document.createTextNode("Image " + im.container + ": "));
      li.appendChild(mono(im.before ? tag(im.before) + " → " + tag(im.after) : "added " + tag(im.after)));
    });
    var more = (r.changed || []).map(function (c) { return CHANGED[c] || c; });
    if (more.length) ul.appendChild(KW.el("li", "", "Also changed: " + more.join(", ") + "."));
    if (r.cause) ul.appendChild(KW.el("li", "", "Change cause: " + r.cause));
    if (r.actor || r.app) ul.appendChild(KW.el("li", "", "By " + (r.app || r.actor) + "."));
    return ul.children.length ? ul : null;
  }

  /* tag: an image without its registry path, "api:1.5". */
  function tag(image) {
    var s = String(image || "");
    return s.length > 48 ? s.slice(s.lastIndexOf("/") + 1) : s;
  }

  function rate(n) { return (Math.round(n * 10) / 10).toLocaleString("en-US"); }

  /* proData: the restarts and the first fatal line, only when kept. */
  function proData(box, d) {
    if (!d) return;
    if (typeof d.restartsAfter === "number") {
      var text = "Restarts per hour: " + rate(d.restartsAfter) + " after";
      if (typeof d.restartsBefore === "number") text += ", " + rate(d.restartsBefore) + " before";
      box.appendChild(KW.el("p", "rl-line", text + "."));
    }
    if (d.errorLine) {
      box.appendChild(KW.el("p", "rl-line", "First fatal line of a new container:"));
      var log = box.appendChild(KW.el("div", "logs rl-log"));
      log.tabIndex = 0;
      log.setAttribute("role", "region");
      log.setAttribute("aria-label", "First fatal line");
      log.appendChild(KW.el("div", "e", d.errorLine));
    }
  }

  function undo(box, u) {
    if (!u || (!u.text && !u.command)) return;
    box.appendChild(KW.el("h4", "rl-h", "To undo"));
    if (u.command) {
      box.appendChild(KW.el("p", "rl-line", "Run this in your terminal. kwatch does not change your cluster."));
      box.appendChild(KW.cmd(u.command));
    } else {
      box.appendChild(KW.el("p", "rl-line", /[.!?]$/.test(u.text) ? u.text : u.text + "."));
    }
  }

  /* KW.releaseRow fills the summary's release row, or hides it. */
  KW.releaseRow = function (root, inc) {
    var row = KW.$('[data-ds-row="release"]', root);
    if (!row) return;
    var r = inc.release, v = r && (VERDICTS[r.verdict] || VERDICTS.unknown);
    row.hidden = !r;
    if (!r) return;
    KW.$("dt", row).textContent = v[3] ? "Likely caused by a release" : "A release near this issue";
    var dd = KW.$("dd", row);
    dd.textContent = "";
    var head = dd.appendChild(KW.el("p", "rl-who"));
    head.appendChild(mono(who(r)));
    dd.appendChild(timing(r, inc));
    var verdict = dd.appendChild(KW.el("p", "rl-verdict"));
    verdict.appendChild(KW.chip(v[0], v[1]));
    verdict.appendChild(document.createTextNode(" " + v[2]));
    var list = changes(r);
    if (list) {
      dd.appendChild(KW.el("h4", "rl-h", "What changed"));
      dd.appendChild(list);
    }
    proData(dd, r.detail);
    undo(dd, r.undo);
  };
})();
