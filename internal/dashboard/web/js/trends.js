/* kwatch dashboard: parts of the Overview's "This week": the last
   seven days as a column chart (opened, with the part that paged
   someone, and resolved, per day, from /api/insights days) with the
   same numbers as a table, the issues that kept coming back and how
   long fixing took (from /api/trends). ovinsights.js reads the data
   and places them. */
(function () {
  "use strict";
  var KW = window.KW;
  var V = KW.viz;
  var WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

  function weekday(date, last) {
    if (last) return "Today";
    var p = date.split("-");
    return WEEKDAYS[new Date(Date.UTC(+p[0], +p[1] - 1, +p[2])).getUTCDay()];
  }

  /* KW.duration: seconds as "under a minute", "12 min", "3 h 10 min",
     "2 days". (KW.lasted takes times; the Unix epoch itself is not a
     valid one, so count from a second after it.) */
  KW.duration = function (sec) {
    if (!(sec >= 60)) return "under a minute";
    return KW.lasted(new Date(1000).toISOString(), new Date(1000 + sec * 1000).toISOString());
  };

  function table(rows, untold) {
    var d = KW.el("details", "ip-table");
    d.appendChild(KW.el("summary", "", "Show as a table"));
    var tb = d.appendChild(KW.el("table", "ip-t"));
    tb.appendChild(KW.el("caption", "sr-only", "Issues per day, last 7 days"));
    var hr = tb.appendChild(KW.el("thead")).appendChild(KW.el("tr"));
    ["Day", "Opened", "Paged", untold ? KW.UNTOLD.label : "", "Resolved"].forEach(function (h, i) {
      if (h) hr.appendChild(KW.el("th", i ? "num" : "", h)).scope = "col";
    });
    var body = tb.appendChild(KW.el("tbody"));
    rows.forEach(function (r) {
      var tr = body.appendChild(KW.el("tr"));
      tr.appendChild(KW.el("th", "", r.label + " " + r.date.slice(5))).scope = "row";
      (untold ? [r.opened, r.paged, r.untold, r.resolved] : [r.opened, r.paged, r.resolved])
        .forEach(function (n) { tr.appendChild(KW.el("td", "num", KW.num(n))); });
    });
    return d;
  }

  /* KW.weekCard: the week's chart, its key and its table; the totals
     are in the numbers above it (ovinsights.js). */
  KW.weekCard = function (days, t) {
    var c = V.card("Day by day", "", true);
    // Untold openings (settled before alerting) are in opened, never in
    // paged; a day without its own count takes /api/trends' for that date.
    var opened = 0, resolved = 0, untold = 0, byDate = {};
    ((t && t.days) || []).forEach(function (d) { byDate[d.date] = d.untold || 0; });
    var rows = days.map(function (d, i) {
      var u = typeof d.untold === "number" ? d.untold : byDate[d.date] || 0;
      opened += d.opened; resolved += d.resolved; untold += u;
      var label = weekday(d.date, i === days.length - 1);
      return { date: d.date, label: label, opened: d.opened, resolved: d.resolved, paged: d.paged, untold: u,
        title: label + " (" + d.date + "): " + KW.num(d.opened) + " opened, " + KW.num(d.paged) +
          " paged the on-call person, " + (u ? KW.num(u) + " settled before alerting, " : "") + KW.num(d.resolved) + " resolved" };
    });
    if (!opened && !resolved) {
      c.appendChild(V.empty("A quiet week: nothing opened or resolved."));
      return c;
    }
    if (untold) {
      c.appendChild(KW.el("p", "pcard-note", KW.num(untold) + " of the " + KW.num(opened) +
        " opened settled before alerting."));
    }
    var spoken = rows.map(function (r) {
      return r.label + ": " + KW.num(r.opened) + " opened, " + KW.num(r.resolved) + " resolved";
    });
    var wrap = c.appendChild(KW.el("div", "ip-chart"));
    wrap.appendChild(V.columns(rows, "Issues per day, last 7 days. " + spoken.join("; ")));
    c.appendChild(V.legend([{ cls: "s-open", label: "Opened" }, { cls: "s-page", label: "Of those, paged the on-call person" },
      { cls: "s-ok", label: "Resolved" }]));
    c.appendChild(table(rows, untold));
    return c;
  };

  function link(href, text) {
    var a = KW.el("a", "tr-link", text);
    a.href = href;
    a.dataset.link = "";
    return a;
  }

  /* KW.keptComingBack: the issues that opened more than once. */
  KW.keptComingBack = function (list) {
    var c = V.card("Kept coming back");
    var ol = KW.el("ol", "tr-list");
    list.forEach(function (n) {
      var li = KW.el("li", "tr-issue");
      KW.clampTitle(li.appendChild(link("/issues/" + encodeURIComponent(n.id), n.title || n.id)));
      var where = KW.el("span", "tr-meta");
      KW.where(n, where);
      li.appendChild(where);
      var words = "Opened " + (n.opened > 1 ? KW.num(n.opened) + " times" : "once") + " in the last 7 days";
      if (n.state === "flapping") words += ", keeps failing and recovering";
      li.appendChild(KW.el("span", "tr-meta", words + "."));
      ol.appendChild(li);
    });
    c.appendChild(ol);
    return c;
  };
})();
