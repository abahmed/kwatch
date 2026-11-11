/* kwatch dashboard: "What kwatch found" on the issue page, under the
   summary rows. Where a dependency's address was read, errors in the
   log of pods Kubernetes sees as healthy, failures that began together,
   the share of the failing pods behind a change, the check that settled
   a close call, whether a resolved issue's cause is still there, and the
   chain of events. Everything is visible text; each term has a one-line
   meaning in its tooltip (on focus too) and in the folded "What does
   this mean?", which stays closed until opened and remembers its state
   in localStorage (kwatch.issueHelp). Hides when the API sends no
   "round". */
(function () {
  "use strict";
  var KW = window.KW;
  var HELP_KEY = "kwatch.issueHelp";

  /* TERMS are the words this block uses and what each means. */
  var TERMS = {
    ConfigMap: "A Kubernetes object that holds settings for pods, such as a hostname.",
    "up but failing": "Kubernetes sees the pod as running and ready, but its own log is reporting errors. The platform check passes; the application is not working.",
    "cause still present": "The app is healthy, but the thing that was breaking it has not recovered. kwatch checked again just now.",
    "same upstream": "Both depend on the same external address or service, so one failure explains both.",
    Ingress: "Routes outside web traffic to Services inside the cluster.",
    "chain of events": "Order in which each step was first seen failing. It shows sequence, not proof of cause.",
    "kwatch checked": "A read-only test kwatch ran from inside the cluster to tell two possible causes apart. It does not change anything.",
    "failing pods read it": "How many of the failing pods use this object. A high share means the change touches most of the failure."
  };

  /* term writes a term with its tooltip; it takes focus, so the
     tooltip opens from the keyboard too. */
  function term(parent, name, label) {
    var s = parent.appendChild(KW.el("span", "rd-term", label || name));
    s.title = TERMS[name];
    s.tabIndex = 0;
    s.setAttribute("aria-label", (label || name) + ". " + TERMS[name]);
    return s;
  }

  KW.roundTerm = term;

  function line(box, label) {
    var p = box.appendChild(KW.el("p", "rd-line"));
    if (label) p.appendChild(KW.el("b", "rd-k", label + " "));
    return p;
  }

  function config(box, c) {
    var p = line(box, "Where it was read");
    p.appendChild(KW.el("span", "mono", c.address));
    p.appendChild(document.createTextNode(" (from "));
    term(p, "ConfigMap");
    p.appendChild(document.createTextNode(" " + c.name + ", " + c.key + ")"));
    if (c.more > 0) p.appendChild(document.createTextNode(", +" + KW.plural(c.more, "more key", "more keys")));
  }

  function burst(box, b) {
    var p = line(box);
    p.appendChild(document.createTextNode("Kubernetes: Running, Ready " + b.readyPods + "/" + b.desiredPods +
      ". "));
    term(p, "up but failing");
    p.appendChild(document.createTextNode(": " + b.errors + " errors in " + b.windowMin + " min."));
    if (b.line) {
      var q = line(box, "Log");
      q.appendChild(KW.el("span", "mono", "×" + b.count + " " + b.line));
      q.appendChild(document.createTextNode(b.before > 0 ? " (up from " + b.before + " before)" :
        " (no earlier errors seen)"));
    }
  }

  function together(box, t) {
    var p = line(box, "Started together");
    p.appendChild(document.createTextNode(t.workloads + " workloads started failing " +
      (t.seconds === 0 ? "at the same time" : t.seconds + " s apart") + ", under the "));
    term(p, "same upstream");
    p.appendChild(document.createTextNode(" shown above."));
  }

  function blast(box, b) {
    var p = line(box, "Overlap");
    term(p, "failing pods read it", b.count + " of " + b.total + " failing pods " + b.verb + " it");
    p.appendChild(document.createTextNode("."));
  }

  function check(box, c) {
    var p = line(box);
    term(p, "kwatch checked", "Check");
    p.appendChild(document.createTextNode(": " + c.text));
    var q = line(box, "Candidates");
    q.appendChild(document.createTextNode(KW.entityText(c.cause) + " and " + KW.entityText(c.rival)));
  }

  function causeNow(box, state) {
    var p = line(box, "Cause now");
    if (state === "still") {
      term(p, "cause still present", "Still present");
      p.appendChild(document.createTextNode(": the issue resolved, but the cause was still there when kwatch checked."));
    } else {
      p.appendChild(document.createTextNode("Recovered: the cause works again."));
    }
  }

  function chain(box, steps) {
    var h = line(box);
    term(h, "chain of events", "Chain of events");
    var ol = box.appendChild(KW.el("ol", "rd-chain"));
    steps.forEach(function (s) {
      var li = ol.appendChild(KW.el("li"));
      li.setAttribute("aria-label", "step " + s.step + " of " + steps.length);
      var t = KW.el("span", "rd-at", KW.valid(s.at) ? KW.hhmm(new Date(s.at)) : "time unknown");
      li.appendChild(t);
      li.appendChild(document.createTextNode(" "));
      li.appendChild(KW.el("span", "mono", (s.kind === "node" ? "" : s.kind + " ") + s.name));
      li.appendChild(document.createTextNode(" " + s.event));
    });
  }

  /* help is the folded list of every term used above, closed until
     opened, its state kept for this browser. */
  function help(box, used) {
    var d = box.appendChild(KW.el("details", "rd-help"));
    d.appendChild(KW.el("summary", "", "What does this mean?"));
    var dl = d.appendChild(KW.el("dl"));
    used.forEach(function (name) {
      dl.appendChild(KW.el("dt", "", name));
      dl.appendChild(KW.el("dd", "", TERMS[name]));
    });
    try { d.open = window.localStorage.getItem(HELP_KEY) === "open"; } catch (e) { /* private mode */ }
    d.addEventListener("toggle", function () {
      try { window.localStorage.setItem(HELP_KEY, d.open ? "open" : "closed"); } catch (e) { /* private mode */ }
    });
  }

  /* KW.roundBlock draws the block into root, once per render. */
  KW.roundBlock = function (root, inc) {
    var host = KW.$("[data-round]", root);
    if (!host) {
      host = KW.el("section", "rd");
      host.dataset.round = "";
      host.setAttribute("aria-label", "What kwatch found");
      var grid = KW.$(".ds-body .ds-grid", root);
      if (!grid) return;
      grid.parentNode.insertBefore(host, grid.nextSibling);
    }
    host.textContent = "";
    var r = inc.round;
    if (!r) { host.hidden = true; return; }
    host.hidden = false;
    var used = [];
    host.appendChild(KW.el("h3", "rd-h", "What kwatch found"));
    if (r.config) { config(host, r.config); used.push("ConfigMap"); }
    if (r.burst) { burst(host, r.burst); used.push("up but failing"); }
    if (r.together) { together(host, r.together); used.push("same upstream"); }
    if (r.blast) { blast(host, r.blast); used.push("failing pods read it"); }
    if (r.check) { check(host, r.check); used.push("kwatch checked"); }
    if (r.causeNow) { causeNow(host, r.causeNow); if (r.causeNow === "still") used.push("cause still present"); }
    if (r.chain && r.chain.length) { chain(host, r.chain); used.push("chain of events"); }
    if ((inc.traffic || []).some(function (t) { return t.kind === "ingress"; })) used.push("Ingress");
    if (used.length) help(host, used);
  };
})();
