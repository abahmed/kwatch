/* kwatch dashboard: the times kwatch was not running (a restart, a
   cluster that sleeps at night, a lost node), from the notWatching
   spans of the API, so that a chart or a list never shows them as a
   quiet time: the words for a stretch, a marker for lists and
   timelines, a day's coverage for the week chart, and the hatch the
   charts draw over those hours. A hatch never stands alone: the words
   say the same. */
(function () {
  "use strict";
  var KW = window.KW;
  var SVG = "http://www.w3.org/2000/svg";
  /* A day off for less than this is not marked: a restart takes a
     minute or two and says nothing about the day. */
  var OFF_MIN = 15 * 60;

  function secs(span) {
    return Math.max(0, (Date.parse(span.to) - Date.parse(span.from)) / 1000);
  }

  /* KW.offSpanText: "kwatch was not running from 02:10 to 08:40
     (6 h 30 min), so nothing in that time was seen." */
  KW.offSpanText = function (span) {
    return "kwatch was not running from " + KW.when(span.from) + " to " +
      KW.when(span.to) + " (" + KW.duration(secs(span)) +
      "), so nothing in that time was seen.";
  };

  /* KW.offMarker: an element (tag, an li in a list) that says kwatch
     was not running, for where a gap falls in a list of events. */
  KW.offMarker = function (tag, span, cls) {
    var m = KW.el(tag, "off-mark" + (cls ? " " + cls : ""));
    m.dataset.off = span.from;
    m.appendChild(KW.el("span", "off-mark-k", "Not watching"));
    m.appendChild(KW.el("span", "off-mark-t", KW.offSpanText(span)));
    return m;
  };

  /* KW.offBetween: the spans that end after "after" and start before
     "before" (times in ms; null for open ends), newest first, each
     once (seen holds the ones placed already). */
  KW.offBetween = function (spans, after, before, seen) {
    return (spans || []).filter(function (s) {
      var from = Date.parse(s.from), to = Date.parse(s.to);
      if (seen[s.from] || !(to > from)) return false;
      if (after !== null && to <= after) return false;
      if (before !== null && from >= before) return false;
      seen[s.from] = true;
      return true;
    }).sort(function (a, b) { return Date.parse(b.from) - Date.parse(a.from); });
  };

  /* KW.dayOff: how much of a week-chart day kwatch was not running:
     {sec, all, short, words}; sec 0 when nothing to mark. all is set
     when it was off the whole day (so far, for today). */
  KW.dayOff = function (d) {
    var off = d.offSeconds || 0, len = d.seconds || 0;
    if (off < OFF_MIN || !len) return { sec: 0, all: false, short: "", words: "" };
    var all = off >= len - 60;
    return { sec: off, all: all, short: all ? "off" : KW.duration(off) + " off",
      words: all ? "kwatch was not running" : "kwatch was off for " + KW.duration(off) };
  };

  /* KW.weekOff: the week's time off, in seconds, and how many days it
     covers: {off, days, watched}; watched is the days watched, to the
     half day. */
  KW.weekOff = function (days) {
    var off = 0, len = 0;
    (days || []).forEach(function (d) {
      off += KW.dayOff(d).sec;
      len += d.seconds || 0;
    });
    return { off: off, days: (days || []).length,
      watched: Math.round(Math.max(0, len - off) / 43200) / 2 };
  };

  var patterns = 0;
  /* KW.offPattern: adds the hatch to an svg and returns its fill. */
  KW.offPattern = function (s) {
    var id = "kw-off-" + (++patterns);
    var defs = s.appendChild(document.createElementNS(SVG, "defs"));
    var p = defs.appendChild(document.createElementNS(SVG, "pattern"));
    [["id", id], ["width", "6"], ["height", "6"], ["patternUnits", "userSpaceOnUse"],
      ["patternTransform", "rotate(45)"]].forEach(function (a) { p.setAttribute(a[0], a[1]); });
    var l = p.appendChild(document.createElementNS(SVG, "line"));
    [["x1", "0"], ["y1", "0"], ["x2", "0"], ["y2", "6"]].forEach(function (a) {
      l.setAttribute(a[0], a[1]);
    });
    l.setAttribute("class", "viz-off-line");
    return "url(#" + id + ")";
  };
})();
