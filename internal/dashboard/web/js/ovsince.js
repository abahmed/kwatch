/* kwatch dashboard: the Overview's "Since your last visit" strip: "2
   new · 1 resolved · 3 changes", counted from /api/activity over the
   changes after this browser's previous visit (visits.js lastVisit),
   with a link to Activity. Nothing on a first visit; a visit over a
   week ago counts the week kwatch keeps. */
(function () {
  "use strict";
  var KW = window.KW;
  var HOUR = 3600 * 1000;

  /* windowFor: the smallest Activity window that reaches back to at. */
  function windowFor(at) {
    var ago = KW.now() - at;
    if (ago <= HOUR) return "1h";
    if (ago <= 24 * HOUR) return "24h";
    return "7d";
  }

  /* counts: the changes after at, by what they mean to a reader. */
  function counts(events, at) {
    var n = { started: 0, resolved: 0, changed: 0 };
    (events || []).forEach(function (e) {
      if (Date.parse(e.at) <= at) return;
      if (e.kind === "new") n.started++;
      else if (e.kind === "resolved") n.resolved++;
      else n.changed++;
    });
    return n;
  }

  function words(n, more) {
    var parts = [];
    if (n.started) parts.push(KW.num(n.started) + " new");
    if (n.resolved) parts.push(KW.num(n.resolved) + " resolved");
    if (n.changed) parts.push(KW.plural(n.changed, "change", "changes"));
    if (!parts.length) return "nothing new";
    return parts.join(" · ") + (more ? " and more" : "");
  }

  /* KW.sinceStrip makes the strip; read() refreshes it, stop() ends it. */
  KW.sinceStrip = function () {
    var box = KW.el("p", "ov-since");
    box.hidden = true;
    box.setAttribute("role", "status");
    var etag = "", last = null, seq = 0;
    function draw(at, data) {
      box.textContent = "";
      var events = data.events || [];
      var n = counts(events, at), oldest = events[events.length - 1];
      var more = (data.total || 0) > events.length && oldest && Date.parse(oldest.at) > at;
      box.appendChild(KW.el("b", "", "Since your last visit"));
      var when = KW.hm(new Date(at).toISOString());
      box.appendChild(document.createTextNode(" ("));
      box.appendChild(when);
      box.appendChild(document.createTextNode("): " + words(n, more) + ". "));
      var a = box.appendChild(KW.el("a", "ulink", "See what changed →"));
      a.href = "/activity?since=" + windowFor(at);
      a.dataset.link = "";
      box.hidden = false;
    }
    function read() {
      var at = KW.visits && KW.visits.lastVisit();
      if (!at) return Promise.resolve(null);
      var mine = ++seq;
      return KW.api("/api/activity?since=" + windowFor(at) + "&limit=200",
        { etag: etag }).then(function (res) {
        if (mine !== seq) return null;
        if (res.status === 200 && res.data) {
          etag = res.etag;
          last = res.data;
        }
        if (last) draw(at, last);
        return res;
      });
    }
    return { el: box, read: read };
  };
})();
