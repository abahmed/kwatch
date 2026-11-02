/* kwatch dashboard: an issue's state and alert level changes
   (transitions). They become entries of "What happened, step by step"
   and the states strip above it, one segment per state, so every flap
   shows. Without transitions (an issue saved by an older kwatch) the
   strip is drawn from the phases. Sizes are set with CSSOM. */
(function () {
  "use strict";
  var KW = window.KW;

  /* SEG: label and CSS class per state, the label in the state words
     the rest of the site uses (KW.STATE); failing is the open state and
     quiet the resolved one. */
  function word(key, fallback) {
    return KW.STATE[key] ? KW.STATE[key].label : fallback;
  }
  var SEG = {
    settling: [word("settling", "Confirming"), "seg-settling"],
    open: [word("open", "Happening now"), "seg-failing"],
    failing: [word("open", "Happening now"), "seg-failing"],
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

  function span(fromIso, toIso) { return KW.when(fromIso) + "–" + KW.when(toIso) + " (" + KW.lasted(fromIso, toIso) + ")"; }

  KW.strip = function (root, inc) {
    var box = KW.$("[data-strip]", root);
    var nowIso = new Date(KW.now()).toISOString();
    var data = fromTransitions(inc, nowIso);
    if (!data.segs.length) data = fromPhases(inc, nowIso);
    var segs = data.segs;
    box.textContent = "";
    box.hidden = !segs.length;
    if (box.hidden) return;
    var start = Date.parse(segs[0].from), stop = Date.parse(segs[segs.length - 1].to);
    var total = Math.max(1, stop - start);
    var bar = KW.el("div", "strip-bar");
    bar.setAttribute("role", "img");
    var words = [];
    segs.forEach(function (p) {
      var ms = Math.max(1, Date.parse(p.to) - Date.parse(p.from));
      var w = SEG[p.state] || [p.state, "seg-quiet"];
      var seg = bar.appendChild(KW.el("span", "seg " + w[1]));
      seg.style.flexGrow = String(ms);
      seg.title = w[0] + ": " + span(p.from, p.to);
      words.push(w[0] + " " + KW.lasted(p.from, p.to));
    });
    var wrap = KW.el("div", "strip-wrap");
    wrap.appendChild(bar);
    data.marks.forEach(function (m) {
      var at = Date.parse(m.at);
      if (at < start || at > stop) return;
      var mk = wrap.appendChild(KW.el("span", "strip-mark"));
      mk.style.left = ((at - start) / total * 100).toFixed(2) + "%";
      mk.title = KW.when(m.at) + " · " + m.text;
      words.push(m.text + " at " + KW.when(m.at));
    });
    bar.setAttribute("aria-label", "States over time: " + words.join(", "));
    var resolved = inc.state === "resolved";
    box.appendChild(KW.el("p", "strip-h", "Over its lifetime" + (data.marks.length ? " · ▲ alert level changed" : "")));
    box.appendChild(wrap);
    var ends = KW.el("div", "strip-ends");
    ends.appendChild(KW.el("span", "", KW.when(segs[0].from)));
    ends.appendChild(KW.el("span", "", resolved ? KW.when(segs[segs.length - 1].to) : "now"));
    box.appendChild(ends);
    var legend = KW.el("p", "strip-key"), seen = {};
    segs.forEach(function (p) {
      var w = SEG[p.state] || [p.state, "seg-quiet"];
      if (seen[w[0]]) return;
      seen[w[0]] = true;
      var item = legend.appendChild(KW.el("span"));
      item.appendChild(KW.el("span", "seg-dot " + w[1]));
      item.appendChild(document.createTextNode(w[0]));
    });
    box.appendChild(legend);
    var cut = KW.cutCount(inc, "phases");
    if (cut) box.appendChild(KW.el("p", "ev-cap", KW.plural(cut, "more phase", "more phases") + " not shown."));
  };
})();
