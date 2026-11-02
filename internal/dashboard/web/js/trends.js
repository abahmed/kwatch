/* kwatch dashboard: "This week" on the Overview, folded away: what the
   last seven days looked like, from /api/trends: issues opened per
   day, the busiest namespaces, the issues that kept coming back and
   how long fixing took (the top causes come from ovinsights.js).
   Read-only; hidden while there is nothing to show. It reads the
   trends with their ETag, so an unchanged answer costs a 304. */
(function () {
  "use strict";
  var KW = window.KW;
  var WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  var SEVS = [["page", "page"], ["notify", "notify"], ["digest", "low"]];

  /* tz: minutes east of UTC that the server counts days in, the zone
     the dashboard shows times in. */
  function tz() {
    return KW.utc ? 0 : -new Date(KW.now()).getTimezoneOffset();
  }

  /* zone: the time zone query: the offset and the zone's name (so
     daylight saving is right); "UTC" while the page shows UTC. */
  function zone(z) {
    var name = "UTC";
    try { if (!KW.utc) name = Intl.DateTimeFormat().resolvedOptions().timeZone || ""; } catch (e) { name = ""; }
    return "tz=" + z + (name ? "&tzName=" + encodeURIComponent(name) : "");
  }

  function weekday(date, last) {
    if (last) return "Today";
    var p = date.split("-");
    return WEEKDAYS[new Date(Date.UTC(+p[0], +p[1] - 1, +p[2])).getUTCDay()];
  }

  function card(title) {
    var c = KW.el("div", "tr-card");
    c.appendChild(KW.el("h3", "tr-ch", title));
    return c;
  }

  /* duration: seconds as "12 min", "3 h 10 min", "2 days". */
  function duration(sec) {
    return KW.lasted(new Date(0).toISOString(), new Date(sec * 1000).toISOString());
  }

  function weekLine(t) {
    var line = KW.plural(t.opened, "issue", "issues") + " opened in the last 7 days";
    if (typeof t.previousWeek !== "number") return line + ".";
    var diff = t.opened - t.previousWeek;
    if (!diff) return line + ", the same as the week before.";
    return line + ", " + KW.num(Math.abs(diff)) + (diff > 0 ? " more" : " fewer") +
      " than the week before.";
  }

  function dayTitle(d, label) {
    var parts = [];
    SEVS.forEach(function (s) {
      if (d[s[0]]) parts.push(KW.num(d[s[0]]) + " " + KW.SEVERITY[s[0]].label.toLowerCase());
    });
    return label + " (" + d.date + "): " + KW.plural(d.total, "issue", "issues") +
      " opened" + (parts.length ? " (" + parts.join(", ") + ")" : "");
  }

  function bars(t) {
    var c = card("Issues opened per day");
    c.appendChild(KW.el("p", "tr-sum", weekLine(t)));
    var max = 1;
    t.days.forEach(function (d) { max = Math.max(max, d.total); });
    var chart = KW.el("div", "tr-bars");
    chart.setAttribute("role", "img");
    var spoken = [];
    t.days.forEach(function (d, i) {
      var label = weekday(d.date, i === t.days.length - 1);
      spoken.push(label + " " + KW.num(d.total));
      var col = KW.el("div", "tr-col");
      col.title = dayTitle(d, label);
      col.appendChild(KW.el("span", "tr-n", d.total ? KW.num(d.total) : ""));
      var track = KW.el("div", "tr-track");
      var stack = KW.el("div", "tr-stack");
      stack.style.height = (d.total / max * 100) + "%";
      SEVS.forEach(function (s) {
        if (!d[s[0]]) return;
        var seg = KW.el("div", "tr-seg " + s[1]);
        seg.style.flexGrow = String(d[s[0]]);
        stack.appendChild(seg);
      });
      track.appendChild(stack);
      col.appendChild(track);
      col.appendChild(KW.el("span", "tr-day", label));
      chart.appendChild(col);
    });
    chart.setAttribute("aria-label", "Issues opened per day: " + spoken.join(", "));
    c.appendChild(chart);
    var key = KW.el("p", "tr-key");
    SEVS.forEach(function (s) {
      var k = KW.el("span");
      k.appendChild(KW.el("span", "dot " + s[1]));
      k.appendChild(document.createTextNode(KW.SEVERITY[s[0]].label));
      key.appendChild(k);
    });
    c.appendChild(key);
    c.appendChild(KW.el("p", "muted small",
      "An issue that came back counts again on the day it came back. Colors are each issue's alert level now."));
    return c;
  }

  function link(href, text) {
    var a = KW.el("a", "tr-link", text);
    a.href = href;
    a.dataset.link = "";
    return a;
  }

  function namespaces(list) {
    var c = card("Busiest namespaces");
    var ol = KW.el("ol", "tr-list");
    var max = list.length ? list[0].count : 1;
    list.forEach(function (n) {
      var li = KW.el("li", "tr-ns");
      var href = "/issues?status=" + (n.active ? "active" : "resolved") +
        "&namespace=" + encodeURIComponent(n.namespace);
      var a = link(href, n.namespace);
      a.title = n.active ? "Show its active issues" : "Show its resolved issues";
      li.appendChild(a);
      li.appendChild(KW.el("span", "tr-meta", "Issues opened " + KW.plural(n.count, "time", "times") +
        (n.active ? ", " + KW.num(n.active) + " still active" : "")));
      var bar = KW.el("span", "tr-meter");
      var fill = KW.el("span", "tr-fill");
      fill.style.width = (n.count / max * 100) + "%";
      bar.appendChild(fill);
      li.appendChild(bar);
      ol.appendChild(li);
    });
    c.appendChild(ol);
    return c;
  }

  function noisiest(list) {
    var c = card("Kept coming back");
    var ol = KW.el("ol", "tr-list");
    list.forEach(function (n) {
      var li = KW.el("li", "tr-issue");
      li.appendChild(link("/issues/" + encodeURIComponent(n.id), n.title || n.id));
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
  }

  function resolve(r) {
    var c = card("Time to resolve");
    c.appendChild(KW.el("p", "tr-sum", KW.plural(r.count, "issue", "issues") +
      " resolved in the last 7 days."));
    var dl = KW.el("dl", "tr-facts");
    dl.appendChild(KW.el("dt", "", "Median"));
    dl.appendChild(KW.el("dd", "", duration(r.medianSeconds)));
    dl.appendChild(KW.el("dt", "", "Longest"));
    var dd = KW.el("dd");
    if (r.longestId) dd.appendChild(link("/issues/" + encodeURIComponent(r.longestId), duration(r.longestSeconds)));
    else dd.textContent = duration(r.longestSeconds);
    dl.appendChild(dd);
    c.appendChild(dl);
    return c;
  }

  /* weekShown shows "This week" when the trends or the top causes
     (ovinsights.js) have something in it. */
  KW.weekShown = function (main) {
    var box = KW.$("[data-trends]", main);
    if (!box) return;
    box.hidden = !KW.$$("[data-trends-body] > *, [data-causes] > *", box).length;
  };

  function draw(box, t) {
    var grid = KW.$("[data-trends-body]", box);
    grid.textContent = "";
    KW.weekShown(box.parentNode);
    if (!t || !t.enough) return;
    grid.appendChild(bars(t));
    if (t.namespaces && t.namespaces.length) grid.appendChild(namespaces(t.namespaces));
    if (t.noisiest && t.noisiest.length) grid.appendChild(noisiest(t.noisiest));
    if (t.resolve && t.resolve.count) grid.appendChild(resolve(t.resolve));
    KW.weekShown(box.parentNode);
  }

  /* trendsView fills the Overview's [data-trends] box; the Overview
     calls load on its own refresh. A failed load keeps what is shown:
     the Overview reports errors itself. */
  KW.trendsView = function (main) {
    var box = KW.$("[data-trends]", main);
    var stopped = false, etag = "", zoneNow = null;
    return {
      load: function () {
        if (!box) return Promise.resolve();
        var z = tz();
        if (z !== zoneNow) etag = "";
        zoneNow = z;
        return KW.api("/api/trends?" + zone(z), { etag: etag }).then(function (res) {
          if (stopped || res.status !== 200 || !res.data) return;
          etag = res.etag;
          draw(box, res.data);
        });
      },
      stop: function () { stopped = true; }
    };
  };
})();
