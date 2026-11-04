/* kwatch dashboard: where an issue is, for the top of the issue page
   (detail.js): the stepper Confirming → Happening now → Looks better →
   Resolved and the line under it. A step before the current one is
   ticked only when the issue really was in it: the states its
   transitions reached. Without them (an issue saved by an older
   kwatch) it is told from its state. */
(function () {
  "use strict";
  var KW = window.KW;

  /* STEP_OF: the step of each state; STEPS the steps in order with the
     state whose words they use. */
  var STEP_OF = { settling: "confirming", open: "happening", flapping: "happening",
    failing: "happening", recovering: "recovering", resolved: "resolved" };
  var STEPS = [["confirming", "settling"], ["happening", "open"], ["recovering", "recovering"],
    ["resolved", "resolved"]];
  var ORDER = STEPS.map(function (s) { return s[0]; });

  function stage(inc) {
    var s = (inc.summary || {}).stage || {};
    return { step: s.step || STEP_OF[inc.state] || "happening", since: s.since || "",
      flapping: !!s.flapping || inc.state === "flapping", stopped: !!s.stoppedTracking || !!inc.stoppedTracking };
  }

  /* reached: the steps the transitions say the issue was in, by index;
     null when they record no state change. Kept transitions may have
     lost the oldest ones, so with a cut the first step counts too. */
  function reached(inc) {
    var out = {}, any = false;
    (inc.transitions || []).forEach(function (t) {
      [t.from, t.to].forEach(function (s) {
        var at = ORDER.indexOf(STEP_OF[s]);
        if (at < 0) return;
        out[at] = true;
        any = true;
      });
    });
    if (!any) return null;
    if (KW.cutCount(inc, "transitions")) out[0] = true;
    return out;
  }

  /* fallback: whether step i (before the current one) happened, for an
     issue without state transitions. An issue settled before alerting
     went from Confirming to Resolved; one kwatch stopped tracking never
     looked better. */
  function fallback(inc, st, i) {
    if (KW.isUntold(inc)) return i === 0;
    return !(i === 2 && st.stopped);
  }

  function stageNote(inc, st) {
    if (KW.isUntold(inc)) return KW.UNTOLD.line;
    if (st.step === "confirming") return "kwatch is checking it's real before it alerts anyone.";
    if (st.flapping && st.step === "happening") return "It keeps failing and recovering. kwatch stays quiet unless it changes.";
    if (st.step === "recovering") return "It looks fixed. kwatch waits a little to be sure.";
    if (st.step === "resolved" && !st.stopped && KW.valid(inc.resolvedAt)) {
      return "It stopped failing " + KW.stamp(inc.resolvedAt) + ".";
    }
    return "";
  }

  /* KW.issueStepper draws the stepper into root's [data-ds-steps], the
     current step marked; flapping and stopped tracking say so. */
  KW.issueStepper = function (root, inc) {
    var st = stage(inc), ol = KW.$("[data-ds-steps]", root);
    var at = ORDER.indexOf(st.step), seen = reached(inc);
    ol.textContent = "";
    STEPS.forEach(function (s, i) {
      var label = KW.STATE[s[1]].label, cls = "ds-step", mark = "";
      if (i === 1 && st.flapping && at === 1) { label = KW.STATE.flapping.label; cls += " is-flap"; mark = "↻"; }
      if (i === 3 && st.stopped) { label = "Stopped tracking"; cls += " is-stop"; mark = "!"; }
      var done = i < at && (seen ? !!seen[i] : fallback(inc, st, i));
      var skip = i < at && !done;
      cls += skip ? " is-next is-skip" : done ? " is-done" : i === at ? " is-now" : " is-next";
      var li = ol.appendChild(KW.el("li", cls));
      li.appendChild(KW.el("span", "ds-dot", done ? "✓" : i === at ? mark || "●" : "")).setAttribute("aria-hidden", "true");
      li.appendChild(KW.el("span", "ds-step-t", label));
      li.appendChild(KW.el("span", "sr-only", skip ? " (skipped)" : done ? " (passed)" : i === at ? " (now)" : " (not yet)"));
      if (skip) li.title = "It never was in this step.";
      if (i !== at) return;
      li.setAttribute("aria-current", "step");
      if (KW.valid(st.since)) li.appendChild(KW.el("small", "ds-since", "since ")).appendChild(KW.hm(st.since));
    });
    var note = KW.$("[data-ds-note]", root), text = stageNote(inc, st);
    note.textContent = text;
    note.hidden = !text;
  };
})();
