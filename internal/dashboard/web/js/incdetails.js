/* kwatch dashboard: the Details tab of the issue page. "Has this
   happened before?" (how often, how long it lasts, whether a change
   fixed it, a strip of the last 30 days and the last five times),
   "Is it happening anywhere else?" (the issues with the same error)
   and "Reference" (the ID and the last change). API text is set with
   textContent only. */
(function () {
  "use strict";
  var KW = window.KW, FIX = KW.TERMS.fix;
  var SHOWN = 5, DAY = 86400000;

  function ms(iso) { return Date.parse(iso); }

  /* changed: how an occurrence ended when a change fixed it. */
  var CHANGED = { rollback: 1, "config fix": 1, change: 1, "node replaced": 1 };

  function median(list) {
    var a = list.slice().sort(function (x, y) { return x - y; }), n = a.length;
    return n % 2 ? a[(n - 1) / 2] : (a[n / 2 - 1] + a[n / 2]) / 2;
  }

  /* typical: "8 min", the middle length of the times that ended. */
  function typical(recent) {
    var spans = recent.filter(function (o) { return KW.valid(o.resolved); })
      .map(function (o) { return ms(o.resolved) - ms(o.opened); });
    if (!spans.length) return "";
    return KW.lasted(new Date(0).toISOString(), new Date(median(spans)).toISOString());
  }

  /* folded: the occurrences newest first, those in the same minute that
     ended the same way as one entry with a count. */
  function folded(recent) {
    var out = [];
    recent.forEach(function (o) {
      var key = KW.stamp(o.opened) + "|" + (o.fix || "") + "|" + (KW.valid(o.resolved) ? "r" : "");
      var last = out[out.length - 1];
      if (last && last.key === key) last.n += 1;
      else out.push({ key: key, o: o, n: 1 });
    });
    return out;
  }

  function entry(g) {
    var o = g.o, li = KW.el("li");
    li.appendChild(KW.el("span", "mono", KW.stamp(o.opened)));
    var parts = [];
    if (KW.valid(o.resolved)) parts.push("lasted " + KW.lasted(o.opened, o.resolved));
    if (FIX[o.fix]) parts.push(FIX[o.fix]);
    if (parts.length) li.appendChild(document.createTextNode(" · " + parts.join(" · ")));
    if (g.n > 1) li.appendChild(KW.el("span", "seen-x", " ×" + g.n)).title = g.n + " times in the same minute";
    li.title = KW.full(o.opened);
    return li;
  }

  function strip(s, now) {
    var items = s.recent.filter(function (o) { return ms(o.opened) >= now - 30 * DAY; }).map(function (o) {
      return { at: o.opened, cls: CHANGED[o.fix] ? "is-fix" : "",
        text: KW.stamp(o.opened) + (CHANGED[o.fix] ? " · " + FIX[o.fix] : "") };
    });
    if (!items.length || !KW.viz || !KW.viz.dots) return null;
    var box = KW.el("div", "seen-strip");
    box.appendChild(KW.viz.dots(items, now - 30 * DAY, now, "Times it failed the same way in the last 30 days", ["30 days ago", "now"]));
    return box;
  }

  KW.seenBefore = function (root, inc) {
    var fold = KW.$('[data-fold="seen"]', root), box = KW.$("[data-seen]", root), s = inc.seenBefore;
    box.textContent = "";
    fold.hidden = !s || !s.recent || !s.recent.length;
    if (fold.hidden) return;
    var n = s.inWindow, typ = typical(s.recent);
    var head = n ? "Yes: " + KW.plural(n, "time", "times") + " in the last " + s.windowDays + " days." :
      "Yes, but not in the last " + s.windowDays + " days.";
    box.appendChild(KW.el("p", "seen-h", head + (typ ? " It usually lasts about " + typ + "." : "")));
    var fixed = s.recent.filter(function (o) { return CHANGED[o.fix]; });
    if (fixed.length) {
      box.appendChild(KW.el("p", "notice n-success seen-fix", "Fixed by a change " +
        (fixed.length === 1 ? "once: " + KW.stamp(fixed[0].resolved || fixed[0].opened) :
          KW.plural(fixed.length, "time", "times") + ", the last " + KW.stamp(fixed[0].resolved || fixed[0].opened))));
    }
    var dots = strip(s, KW.now());
    if (dots) box.appendChild(dots);
    var rows = folded(s.recent), open = box.dataset.all === "1";
    var ul = box.appendChild(KW.el("ul", "seen-list"));
    (open ? rows : rows.slice(0, SHOWN)).forEach(function (g) { ul.appendChild(entry(g)); });
    if (rows.length > SHOWN) {
      var b = box.appendChild(KW.el("button", "btn btn-sm seen-all", open ? "Show fewer" :
        "Show all " + KW.num(s.recent.length)));
      b.title = KW.num(rows.length) + " lines: times in the same minute are one line, marked with ×";
      b.type = "button";
      b.addEventListener("click", function () {
        box.dataset.all = open ? "" : "1";
        KW.seenBefore(root, inc);
      });
    }
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
  };

  /* KW.similar: "Is it happening anywhere else?": how many other issues
     have the same error, three of them by name, and the link to all. */
  KW.similar = function (root, inc) {
    var fold = KW.$('[data-fold="similar"]', root), body = KW.$(".fold-body", fold), n = inc.similar || 0;
    fold.hidden = !n;
    body.textContent = "";
    if (!n) return;
    body.appendChild(KW.el("p", "seen-h", n === 1 ? "Yes: 1 other issue has the same error." :
      "Yes: " + KW.num(n) + " other issues have the same error."));
    var list = inc.similarIssues || [];
    if (list.length) {
      var ul = body.appendChild(KW.el("ul", "rel-list"));
      list.forEach(function (r) {
        var li = ul.appendChild(KW.el("li", "rel-row"));
        li.appendChild(KW.stateChip(r.state));
        var a = KW.el("a", "ulink", KW.shortTitle ? KW.shortTitle(r.title) : r.title);
        a.href = "/issues/" + encodeURIComponent(r.id);
        a.dataset.link = "";
        a.title = r.title;
        li.appendChild(KW.el("div", "rel-body")).appendChild(a);
      });
    }
    var all = body.appendChild(KW.el("p", "seen-prev")).appendChild(KW.el("a", "ulink",
      "See all " + KW.num(n) + " →"));
    all.setAttribute("href", "/issues?status=all&like=" + encodeURIComponent(inc.id));
    all.dataset.link = "";
    all.title = "Issues that failed the same way, with the same likely cause, in any namespace. Active and resolved.";
  };

  /* lastChange: the newest sign of change: the API's last seen, or a
     later timeline entry or state change. */
  KW.lastChange = function (inc) {
    var best = KW.valid(inc.lastSeen) ? ms(inc.lastSeen) : 0;
    (inc.timeline || []).concat(inc.transitions || []).forEach(function (e) {
      if (KW.valid(e.at) && ms(e.at) > best) best = ms(e.at);
    });
    return best ? new Date(best).toISOString() : "";
  };

  function oneLine(iso) {
    var t = KW.el("time", "d-nowrap", KW.stamp(iso) + (KW.utc ? " UTC" : ""));
    t.setAttribute("datetime", iso);
    t.title = KW.full(iso);
    return t;
  }

  /* KW.facts: "Reference": the issue's ID with Copy and its times. */
  KW.facts = function (dl, inc) {
    dl.textContent = "";
    function add(label, value, cls, tip) {
      var dt = dl.appendChild(KW.el("dt", "", label));
      if (tip) dt.title = tip;
      var dd = dl.appendChild(KW.el("dd", cls || ""));
      if (typeof value === "string") dd.textContent = value;
      else dd.appendChild(value);
      return dd;
    }
    var id = add("Issue ID", KW.el("span", "mono", inc.id)), b = id.appendChild(KW.el("button", "btn btn-sm ds-copy", "Copy"));
    b.type = "button";
    b.addEventListener("click", function () { KW.copy(b, inc.id, "Copy"); });
    var last = KW.lastChange(inc);
    if (last) add("Last change", KW.timeEl(last, "", false), "", "The newest thing kwatch saw happen: " + KW.full(last));
    add("Started", oneLine(inc.firstSeen));
    if (inc.state === "resolved" && KW.valid(inc.resolvedAt)) {
      add(inc.stoppedTracking ? "Stopped tracking" : "Resolved", KW.stamp(inc.resolvedAt) + " (lasted " +
        KW.lasted(inc.firstSeen, inc.resolvedAt) + ")");
    }
    if (inc.ackedBy) {
      add("Acknowledged", "Yes, on " + inc.ackedBy, "mono",
        "Someone marked this as seen with kwatch's annotation on that object, so kwatch stops reminding.");
    }
  };
})();
