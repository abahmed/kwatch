/* kwatch dashboard: "Alert status", under the
   summary card. Why it is still open (with a bar that fills each second
   while kwatch waits for it to stay healthy), why it alerted at its
   level, how fast kwatch saw and told, and the acknowledgement, or the
   command that sets it, and the pager's side: the dashboard itself only
   reads. */
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
    p.appendChild(KW.el("b", "", inc.state === "resolved" ? "Why it alerted: " : "Why you got this alert: "));
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
      part(p, "told " + (t.toldVia || "a channel"), t.toldAt,
        "When an alert channel first accepted a message about it:");
    }
    p.hidden = !p.firstChild;
  }

  /* extra: lines the template has no place for, in the alert box: that
     kwatch can't see the object, and the pager. Rebuilt on every draw. */
  var PAGER = {
    open: ["still open on the pager", ""], closed: ["closed", "closedAt"],
    pending: ["page pending", ""], failed: ["page failed", ""]
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
    p.textContent = "Paged " + name + bits.join("") + (tail ? " · " + tail : "");
    if (KW.valid(g.pagedAt)) p.title = KW.full(g.pagedAt);
    return p;
  }

  function extra(root, inc, why) {
    var box = KW.$("[data-extra]", root);
    if (!box) {
      box = KW.el("div", "al-extra");
      box.dataset.extra = "";
      KW.$("[data-alert-box]", root).appendChild(box);
    }
    box.textContent = "";
    if (why.cannotSee && inc.state !== "resolved") {
      box.appendChild(KW.el("p", "al-line muted",
        "kwatch can't see this right now, so it stays open until it can check again."));
    }
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
    extra(root, inc, why);
    KW.$("[data-alert-box]", root).hidden = ["[data-why-open]", "[data-why-level]", "[data-timing]",
      "[data-ack]", "[data-extra]"].every(function (s) { return KW.$(s, root).hidden; });
  };
})();
