/* kwatch dashboard: an issue's state and alert level changes
   (transitions). They become entries of "What happened, step by step"
   and the states strip above it, one segment per state, so every flap
   shows. Without transitions (an issue saved by an older kwatch) the
   strip is drawn from the phases. The strip is an SVG built with DOM
   calls, placed by percent. */
(function () {
  "use strict";
  var KW = window.KW;

  /* SEG: label and CSS class per state, the label in the state words
     the rest of the site uses (KW.STATE); failing is the open state and
     quiet the resolved one. */
  function word(key, fallback) {
    return KW.STATE[key] ? KW.STATE[key].label : fallback;
  }
  /* Failing is a past red band; only the band still going now is
     "Happening now" (segWord). */
  var SEG = {
    settling: [word("settling", "Confirming"), "seg-settling"],
    open: ["Failing", "seg-failing"],
    failing: ["Failing", "seg-failing"],
    flapping: [word("flapping", "Keeps coming back"), "seg-flapping"],
    recovering: [word("recovering", "Looks better"), "seg-recovering"],
    resolved: [word("resolved", "Resolved"), "seg-quiet"],
    quiet: [word("resolved", "Resolved"), "seg-quiet"]
  };

  /* covered: transitions the timeline already tells in its own words. */
  var COVERED = { flapping: "flapping", resolved: "resolved" };

  function level(name) {
    var s = KW.SEVERITY[name];
    return s ? s.label : name;
  }

  function stateWord(name) {
    var s = KW.STATE[name];
    return s ? s.label : name;
  }

  /* text: a transition in plain words and its timeline class; null for
     the opening, which the timeline already starts with. */
  function text(t) {
    switch (t.kind) {
      case "opened": return null;
      case "announced": return ["Confirmed: kwatch alerted about it", "k-warn"];
      case "recovering": return ["Looked better: kwatch waits to be sure it stays healthy", ""];
      case "failing again": return ["Failed again before it could close", "k-warn"];
      case "flapping": return ["Keeps failing and recovering", "k-warn"];
      case "reopened": return ["Came back after it was resolved", "k-warn"];
      case "resolved": return ["Resolved", "k-done"];
      case "tier raised": return ["Alert level raised: " + level(t.from) + " → " + level(t.to), "k-warn"];
      case "tier lowered": return ["Alert level lowered: " + level(t.from) + " → " + level(t.to), ""];
    }
    return ["State changed: " + stateWord(t.from) + " → " + stateWord(t.to), ""];
  }

  /* KW.transitionShown: whether a transition is its own timeline
     entry, not already told by a timeline note within a minute. */
  KW.transitionShown = function (t, inc) {
    if (!text(t) || !KW.valid(t.at)) return false;
    var kind = COVERED[t.kind];
    if (!kind) return true;
    var at = Date.parse(t.at);
    return !(inc.timeline || []).some(function (e) {
      return e.kind === kind && Math.abs(Date.parse(e.at) - at) < 60000;
    });
  };

  /* KW.transitionEvents: the transitions as timeline events. */
  KW.transitionEvents = function (inc) {
    return (inc.transitions || []).filter(function (t) {
      return KW.transitionShown(t, inc);
    }).map(function (t) {
      var w = text(t);
      return { at: t.at, kind: "transition", text: w[0], cls: w[1] };
    });
  };

  /* segments: the states over time from the transitions, oldest first,
     with the alert level changes as marks. */
  function fromTransitions(inc, nowIso) {
    var list = (inc.transitions || []).filter(function (t) { return KW.valid(t.at); })
      .slice().sort(function (a, b) { return Date.parse(a.at) - Date.parse(b.at); });
    var segs = [], marks = [];
    list.forEach(function (t) {
      if (t.kind === "tier raised" || t.kind === "tier lowered") {
        marks.push({ at: t.at, text: text(t)[0] });
        return;
      }
      var last = segs[segs.length - 1];
      if (last) last.to = t.at;
      if (t.to && t.to !== "resolved") segs.push({ state: t.to, from: t.at, to: "" });
      else if (t.to === "resolved") segs.push({ state: "resolved", from: t.at, to: t.at });
    });
    var end = segs[segs.length - 1];
    if (end && !end.to) end.to = nowIso;
    // A resolved issue ends at its resolve; one still open, now.
    if (end && end.state === "resolved") segs.pop();
    return { segs: segs, marks: marks };
  }

  function fromPhases(inc, nowIso) {
    return {
      segs: (inc.phases || []).map(function (p) {
        return { state: p.state, from: p.from, to: KW.valid(p.to) ? p.to : nowIso };
      }),
      marks: []
    };
  }

  /* segWord: the band's label and class; the last band of an issue
     still open is the current state. */
  function segWord(p, last, inc) {
    var w = SEG[p.state] || [p.state, "seg-quiet"];
    if (last && w[1] === "seg-failing" && inc.state !== "resolved") return [word("open", "Happening now"), w[1]];
    return w;
  }

  function span(fromIso, toIso) { return KW.when(fromIso) + "–" + KW.when(toIso) + " (" + KW.lasted(fromIso, toIso) + ")"; }

  var NS = "http://www.w3.org/2000/svg", H = 40, TOP = 12;
  /* TALL: each state's band height, so states differ by more than color. */
  var TALL = { "seg-failing": 1, "seg-flapping": 0.75, "seg-recovering": 0.5, "seg-settling": 0.3, "seg-quiet": 0.2 };

  function svg(tag, attrs, cls, tip) {
    var n = document.createElementNS(NS, tag);
    Object.keys(attrs).forEach(function (k) { n.setAttribute(k, String(attrs[k])); });
    if (cls) n.setAttribute("class", cls);
    if (tip) n.appendChild(document.createElementNS(NS, "title")).textContent = tip;
    return n;
  }

  /* pin: a mark at a moment, drawn in a nested svg placed by percent so
     the mark keeps its shape at any width. */
  function pin(x, shape, cls, tip) {
    var g = svg("svg", { x: x, y: 0, width: 1, height: H, overflow: "visible" });
    g.appendChild(svg("line", { x1: 0, x2: 0, y1: 6, y2: H }, cls + "-l"));
    g.appendChild(svg("polygon", { points: shape }, cls, tip));
    return g;
  }

  function changes(inc) {
    var list = (inc.changes || []).concat(inc.state === "resolved" ? inc.changedBeforeRecovery || [] : []);
    return list.filter(function (c) { return KW.valid(c.at); });
  }

  /* KW.strip draws the mini timeline above the step-by-step list: the
     states as bands (taller is worse), changes people made as ▼ and
     alert level changes as ◆, with the same in words for screen readers. */
  KW.strip = function (root, inc) {
    var box = KW.$("[data-strip]", root);
    var nowIso = new Date(KW.now()).toISOString();
    var data = fromTransitions(inc, nowIso);
    if (!data.segs.length) data = fromPhases(inc, nowIso);
    var segs = data.segs, chg = changes(inc);
    box.textContent = "";
    box.hidden = !segs.length;
    if (box.hidden) return;
    var start = Date.parse(segs[0].from), stop = Date.parse(segs[segs.length - 1].to);
    chg.forEach(function (c) {
      start = Math.min(start, Date.parse(c.at));
      stop = Math.max(stop, Date.parse(c.at));
    });
    var total = Math.max(1, stop - start);
    function x(at) { return ((Date.parse(at) - start) / total * 100).toFixed(2) + "%"; }
    var s = svg("svg", { width: "100%", height: H, focusable: "false" }, "mt-svg");
    s.setAttribute("role", "img");
    s.appendChild(svg("rect", { x: 0, y: TOP, width: "100%", height: H - TOP }, "mt-bg"));
    var words = [];
    segs.forEach(function (p, i) {
      var w = segWord(p, i === segs.length - 1, inc), h = (H - TOP) * (TALL[w[1]] || 0.2);
      var width = Math.max(0.4, (Date.parse(p.to) - Date.parse(p.from)) / total * 100);
      s.appendChild(svg("rect", { x: x(p.from), y: H - h, width: width.toFixed(2) + "%", height: h },
        "mt-band " + w[1], w[0] + ": " + span(p.from, p.to)));
      words.push(w[0] + " " + KW.lasted(p.from, p.to));
    });
    chg.forEach(function (c) {
      var text = KW.when(c.at) + " · " + KW.obj(c.kind, c.namespace, c.name) + ": " + c.summary;
      s.appendChild(pin(x(c.at), "-5,0 5,0 0,7", c.cause ? "mt-chg is-cause" : "mt-chg",
        text + (c.cause ? " (likely cause)" : "")));
      words.push("change at " + KW.when(c.at) + ": " + c.summary);
    });
    data.marks.forEach(function (m) {
      s.appendChild(pin(x(m.at), "0,0 4,4 0,8 -4,4", "mt-tier", KW.when(m.at) + " · " + m.text));
      words.push(m.text + " at " + KW.when(m.at));
    });
    s.setAttribute("aria-label", "Over its lifetime: " + words.join(", "));
    box.appendChild(KW.el("p", "strip-h", "Over its lifetime" +
      (chg.length ? " · " + KW.plural(chg.length, "change", "changes") + " people made" : "")));
    box.appendChild(s);
    var ends = KW.el("div", "strip-ends");
    ends.appendChild(KW.el("span", "", KW.when(new Date(start).toISOString())));
    ends.appendChild(KW.el("span", "", inc.state === "resolved" ? KW.when(new Date(stop).toISOString()) : "now"));
    box.appendChild(ends);
    var legend = KW.el("p", "strip-key"), seen = {};
    segs.forEach(function (p, i) {
      var w = segWord(p, i === segs.length - 1, inc);
      if (seen[w[0]]) return;
      seen[w[0]] = true;
      var item = legend.appendChild(KW.el("span"));
      item.appendChild(KW.el("span", "seg-dot " + w[1]));
      item.appendChild(document.createTextNode(w[0]));
    });
    if (chg.length) legend.appendChild(KW.el("span", "mt-key-chg", "▼ change"));
    if (data.marks.length) legend.appendChild(KW.el("span", "mt-key-tier", "◆ alert level changed"));
    box.appendChild(legend);
    var cut = KW.cutCount(inc, "phases");
    if (cut) box.appendChild(KW.el("p", "ev-cap", KW.plural(cut, "more phase", "more phases") + " not shown."));
  };
})();
