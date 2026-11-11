/* kwatch dashboard: "Alert status" in the Alerts tab, and the notes
   at the top of the summary card. Why it is still open (shown in the
   summary, with a bar that fills each second while kwatch waits for it
   to stay healthy), why it alerted at its level, how fast kwatch saw
   and told, the acknowledgement or the command that sets it, and the
   pager's side: the dashboard itself only reads. */
(function () {
  "use strict";
  var KW = window.KW;
  var ACK = "kwatch.dev/ack";


  /* seconds of a Go duration such as "1m20s" or "2h"; NaN otherwise. */
  function goSeconds(text) {
    var m = /^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/.exec(text || "");
    if (!m || !text) return NaN;
    return (+m[1] || 0) * 3600 + (+m[2] || 0) * 60 + (+m[3] || 0);
  }

  /* words: "40 s", "1 min 20 s", "3 min", "1 h 5 min". */
  function words(sec) {
    sec = Math.max(0, Math.floor(sec));
    if (sec < 60) return sec + " s";
    var m = Math.floor(sec / 60), s = sec % 60;
    if (m < 60) return m + " min" + (s ? " " + s + " s" : "");
    var h = Math.floor(m / 60), r = m % 60;
    return h + " h" + (r ? " " + r + " min" : "");
  }

  /* plain rewrites kwatch's wording for a newcomer: state names as the
     page shows them and durations in words. */
  function plain(text) {
    return text.replace(/; /g, " · ").replace(/^Flapping:/, "Keeps coming back:")
      .replace(/\b(?:\d+h)?(?:\d+m)?(?:\d+s)?\b/g, function (d) {
        var n = goSeconds(d);
        return isNaN(n) || !/[hms]$/.test(d) ? d : words(n);
      });
  }

  /* lead drops the state word the summary card already shows ("Happening
     now; closes after 3 min healthy" reads "Closes after 3 min healthy"),
     so the card does not say it twice. */
  function lead(text, inc) {
    var st = KW.STATE[inc.state];
    if (!st || text.indexOf(st.label) !== 0) return text;
    var rest = text.slice(st.label.length).replace(/^\s*[:;,·]?\s*/, "");
    return rest.charAt(0).toUpperCase() + rest.slice(1);
  }

  /* hold works out the live part of why it is open: counting up while
     kwatch waits for it to stay healthy (or stable), down while it
     confirms before alerting. null when nothing moves. */
  function hold(inc, why) {
    var text = why.stillOpenReason || "", total = why.holdTotal || 0, done = why.holdElapsed || 0;
    if (total > 0 && done < total && inc.state === "recovering") {
      return { mode: "up", total: total, base: done,
        pre: "Closes after " + words(total) + " healthy · " };
    }
    if (total > 0 && done < total && /^Flapping: waiting/.test(text)) {
      return { mode: "up", total: total, base: done,
        pre: "Closes after " + words(total) + " stable · " };
    }
    var m = /^(.*, )(\S+) left$/.exec(text);
    if (m && !isNaN(goSeconds(m[2]))) {
      return { mode: "down", total: 0, base: goSeconds(m[2]), pre: lead(plain(m[1]), inc) };
    }
    return null;
  }

  function update(box) {
    var d = box.dataset, gone = (KW.now() - Number(d.read)) / 1000;
    var p = KW.$("[data-why-text]", box);
    if (d.hold === "up") {
      var total = Number(d.total), e = Math.min(total, Number(d.base) + gone);
      p.textContent = e >= total ? "Healthy long enough · it can close at the next check" :
        d.pre + words(e) + " so far";
      var bar = KW.$("[data-why-bar]", box);
      bar.setAttribute("aria-valuenow", String(Math.round(e)));
      bar.setAttribute("aria-valuetext", words(e) + " of " + words(total));
      KW.$("[data-why-fill]", box).style.width = (e / total * 100).toFixed(1) + "%";
      return;
    }
    var left = Math.max(0, Number(d.base) - gone);
    p.textContent = d.pre + (left >= 1 ? words(left) + " left" : "any moment now");
  }

  var timer = 0;
  function tick() {
    var boxes = KW.$$("[data-hold]");
    if (!boxes.length) {
      window.clearInterval(timer);
      timer = 0;
      return;
    }
    boxes.forEach(update);
  }

  function stillOpen(root, inc, why) {
    var box = KW.$("[data-why-open]", root);
    var bar = KW.$("[data-why-bar]", root);
    var text = why.stillOpenReason || "";
    box.hidden = !text || inc.state === "resolved" || !!why.cannotSee;
    box.removeAttribute("data-hold");
    bar.hidden = true;
    if (box.hidden) return;
    var h = hold(inc, why);
    KW.$("[data-why-text]", box).textContent = lead(plain(text), inc);
    box.title = "Why kwatch keeps this issue open. It closes on its own; nobody needs to close it.";
    if (!h) return;
    box.dataset.hold = h.mode;
    box.dataset.total = String(h.total);
    box.dataset.base = String(h.base);
    box.dataset.pre = h.pre;
    box.dataset.read = String(KW.now());
    if (h.mode === "up") {
      bar.hidden = false;
      bar.setAttribute("role", "progressbar");
      bar.setAttribute("aria-valuemin", "0");
      bar.setAttribute("aria-valuemax", String(h.total));
      bar.setAttribute("aria-label", "Time it has stayed healthy");
    }
    update(box);
    if (!timer) timer = window.setInterval(tick, 1000);
  }

  function level(root, inc, why) {
    var p = KW.$("[data-why-level]", root);
    p.hidden = !why.alertLevelReason;
    if (p.hidden) return;
    p.textContent = "";
    // With no alert sent (a lowered issue) it says why the level is what it is.
    var sent = KW.firstAlert ? !!KW.firstAlert(inc) || !!(inc.pager || []).length : true;
    p.appendChild(KW.el("b", "", !sent ? "Why this level: " :
      inc.state === "resolved" ? "Why it alerted: " : "Why you got this alert: "));
    p.appendChild(document.createTextNode(why.alertLevelReason));
  }

  function part(p, label, iso, tip) {
    if (p.firstChild) p.appendChild(document.createTextNode(" · "));
    var s = p.appendChild(KW.el("span", "al-t"));
    s.appendChild(document.createTextNode(label + " "));
    // hh:mm like the rest of the page; the seconds are in the tooltip.
    var t = s.appendChild(KW.el("time", "mono", KW.when(iso)));
    t.dateTime = iso;
    s.title = tip + " " + KW.full(iso) + ".";
  }

  /* timing: "Started 12:30 · kwatch noticed 12:30 · told Slack 12:31". */
  function timing(root, inc) {
    var p = KW.$("[data-timing]", root), t = inc.timing || {};
    p.textContent = "";
    if (KW.valid(t.startedAt) && t.startedAt !== t.noticedAt) {
      part(p, "Started", t.startedAt, "When the first failure began, as Kubernetes reported it:");
    }
    if (KW.valid(t.noticedAt)) {
      part(p, "kwatch noticed", t.noticedAt, "When kwatch opened this issue:");
    }
    if (KW.valid(t.toldAt)) {
      part(p, "alerted " + (t.toldVia || "a channel"), t.toldAt,
        "When an alert channel first accepted a message about it:");
    }
    p.hidden = !p.firstChild;
  }

  /* extra: the pager lines the template has no place for, in the
     alert box. Rebuilt on every draw. */
  var PAGER = {
    open: ["still open on the pager", ""], closed: ["closed", "closedAt"],
    pending: ["alert pending", ""], failed: ["alert failed", ""]
  };

  function pagerLine(g) {
    var name = g.name || g.provider || "the pager";
    var p = KW.el("p", "al-line al-pager");
    if (g.state === "failed" || g.state === "pending") {
      p.textContent = name + ": " + PAGER[g.state][0] + (g.state === "failed" && g.reason ? ": " + g.reason : "");
      return p;
    }
    var bits = [];
    if (KW.valid(g.pagedAt)) bits.push(" at " + KW.when(g.pagedAt));
    var tail = g.state === "closed" && KW.valid(g.closedAt) ? "closed at " + KW.when(g.closedAt) :
      (PAGER[g.state] || ["", ""])[0];
    p.textContent = "Alerted " + name + bits.join("") + (tail ? " · " + tail : "");
    if (KW.valid(g.pagedAt)) p.title = KW.full(g.pagedAt);
    return p;
  }

  function extra(root, inc) {
    var box = KW.$("[data-extra]", root);
    if (!box) {
      box = KW.el("div", "al-extra");
      box.dataset.extra = "";
      KW.$("[data-alert-box]", root).appendChild(box);
    }
    box.textContent = "";
    (inc.pager || []).forEach(function (g) { box.appendChild(pagerLine(g)); });
    var cut = KW.cutCount(inc, "pager");
    if (cut) box.appendChild(KW.el("p", "ev-cap", KW.plural(cut, "more page", "more pages") + " not shown."));
    box.hidden = !box.firstChild;
  }

  /* target: kind/name and namespace flag for kubectl, from the object
     that carries the acknowledgement or else the issue's own object. */
  function target(inc) {
    var on = inc.ack && inc.ack.on ? inc.ack.on.split("/") : null;
    if (on && on.length >= 3 && on[0] && on[2]) {
      return { ns: on[1], obj: on[0].toLowerCase() + "/" + on.slice(2).join("/") };
    }
    if (!inc.workloadKind || !inc.workload) return null;
    return { ns: inc.namespace, obj: inc.workloadKind.toLowerCase() + "/" + inc.workload };
  }

  function command(t, value) {
    return "kubectl " + (t.ns ? "-n " + t.ns + " " : "") + "annotate " + t.obj + " " + ACK + value;
  }

  /* KW.ackCommand: the kubectl command that acknowledges the issue,
     or "" when it is acknowledged or has no object to annotate. */
  KW.ackCommand = function (inc) {
    var t = inc.ack ? null : target(inc);
    return t ? command(t, '="on it"') : "";
  };

  function how(summary, items) {
    var det = KW.el("details", "al-how");
    det.dataset.keep = "";
    det.appendChild(KW.el("summary", "", summary));
    items.forEach(function (n) { det.appendChild(n); });
    return det;
  }

  function ack(root, inc) {
    var box = KW.$("[data-ack]", root), a = inc.ack, t = target(inc);
    box.textContent = "";
    box.hidden = inc.state === "resolved" || (!a && !t);
    if (box.hidden) return;
    var p = box.appendChild(KW.el("p", "al-line"));
    if (a) {
      p.appendChild(KW.el("b", "", "Acknowledged"));
      var bits = [];
      if (KW.valid(a.since)) bits.push(" " + KW.when(a.since));
      if (a.by) bits.push(" by " + a.by);
      p.appendChild(document.createTextNode(bits.join("") + (a.note ? " · “" + a.note + "”" : "")));
      if (KW.valid(a.since)) p.title = "kwatch saw it " + KW.full(a.since) + (a.on ? ", on " + a.on : "") + ".";
      if (t) box.appendChild(how("How to remove it", [KW.cmd(command(t, "-")),
        KW.el("p", "ev-cap", "kwatch starts reminding again once it is gone.")]));
      return;
    }
    // The one place the page says nobody has acknowledged it.
    p.appendChild(KW.el("b", "", "Not acknowledged"));
    p.appendChild(document.createTextNode(" · nobody has marked it as seen yet."));
    box.appendChild(how("How to acknowledge it", [
      KW.el("p", "", "Run this where you use kubectl. kwatch posts one update and stops reminding while the annotation is there:"),
      KW.cmd(command(t, '="on it"')),
      KW.el("p", "ev-cap", "To take it back: " + command(t, "-")),
      KW.el("p", "ev-cap", "This page only reads your cluster, so it can't do this for you.")]));
  }

  /* KW.alertBox fills "What's going on with this alert"; it hides when
     it has nothing to say. live updates only why it is still open. */
  KW.alertBox = function (root, inc, live) {
    var why = inc.why || {};
    stillOpen(root, inc, why);
    // A live reading only moves the timer; the rest waits for a redraw.
    if (live) return;
    KW.$("#d-alert-h", root).textContent = "Alert status";
    level(root, inc, why);
    timing(root, inc);
    ack(root, inc);
    extra(root, inc);
    KW.$("[data-alert-box]", root).hidden = ["[data-why-level]", "[data-timing]",
      "[data-ack]", "[data-extra]"].every(function (s) { return KW.$(s, root).hidden; });
  };

  /* Notes icons: small line drawings, one per kind of note. */
  var NOTE_ICON = { warn: "M8 2l7 12H1zM8 6v4M8 11.5v.5", ok: "M3 8.5l3 3 7-7",
    lock: "M4 7h8v7H4zM6 7V5a2 2 0 0 1 4 0v2", caused: "M3 2v7h9M9 6l3 3-3 3",
    moved: "M2 5h10M9 2l3 3-3 3M14 11H4M7 8l-3 3 3 3", short: "M2 8h12M5 5l-3 3 3 3M11 5l3 3-3 3",
    looked: "M1 8s3-5 7-5 7 5 7 5-3 5-7 5-7-5-7-5zM8 6.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3" };
  function noteIcon(kind) {
    var s = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    s.setAttribute("class", "ds-note-ico");
    s.setAttribute("viewBox", "0 0 16 16");
    s.setAttribute("aria-hidden", "true");
    var p = document.createElementNS("http://www.w3.org/2000/svg", "path");
    p.setAttribute("d", NOTE_ICON[kind]);
    s.appendChild(p);
    return s;
  }

  /* KW.summaryNotes: the one muted line of notes under the stepper,
     each at most one line with its full text in its tooltip: kwatch
     can't see it, who acknowledged it, that secrets are hidden. */
  KW.summaryNotes = function (root, inc) {
    var ul = KW.$("[data-ds-dyn]", root), a = inc.ack, why = inc.why || {};
    ul.textContent = "";
    function note(cls, kind, text, tip) {
      var li = ul.appendChild(KW.el("li", "ds-note " + cls));
      li.appendChild(noteIcon(kind));
      li.appendChild(KW.el("span", "", text));
      li.title = tip || text;
    }
    if (why.cannotSee && inc.state !== "resolved") {
      note("n-warn", "warn", "Can't see it right now",
        "kwatch can't see this right now, so it stays open until it can check again.");
    }
    var when = a && KW.valid(a.since) ? " " + KW.when(a.since) : "";
    if (a && inc.state !== "resolved") {
      note("n-ok", "ok", "Acknowledged" + when + (a.by ? " by " + a.by : ""),
        (a.note ? "\u201c" + a.note + "\u201d\n" : "") + "kwatch stops reminding while the annotation is there.");
    } else if (inc.ackedBy && inc.state !== "resolved") {
      note("n-ok", "ok", "Acknowledged on " + inc.ackedBy);
    }
    if (/\[redacted/.test(JSON.stringify(inc.summary || {}))) {
      note("", "lock", "Secrets hidden", "Parts marked hidden are secrets kwatch removed for safety.");
    }
    ul.hidden = !ul.firstChild;
  };

  /* KW.notesRow, once every module filled its note: an icon and a
     tooltip on caused by, moved, changed since last look and shortened
     text; the row hides when it has nothing. */
  KW.notesRow = function (root) {
    var box = KW.$(".ds-notes", root);
    if (!box) return;
    [["[data-caused]", "caused"], ["[data-moved] > p", "moved"], ["[data-looked]", "looked"],
      ["[data-short]", "short"]].forEach(function (k) {
      KW.$$(k[0], box).forEach(function (n) {
        if (n.hidden || !n.firstChild || n.querySelector(".ds-note-ico")) return;
        n.insertBefore(noteIcon(k[1]), n.firstChild);
        n.title = n.textContent + (n.title && n.title !== n.textContent ? "\n" + n.title : "");
      });
    });
    box.hidden = KW.$$("[data-ds-dyn] > li, [data-caused], [data-moved] > p, [data-looked], [data-short]", box)
      .every(function (n) { return n.hidden || !n.isConnected || n.closest("[hidden]"); });
  };
})();
