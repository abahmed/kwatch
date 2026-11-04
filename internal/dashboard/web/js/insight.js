/* kwatch dashboard: what the cluster says now about an issue's
   objects. "Why it's waiting to start", "Failing probe", "Recent
   warnings", "Traffic impact" and "Node". Every part hides when kwatch
   has nothing for it. API text is set with textContent only. */
(function () {
  "use strict";
  var KW = window.KW;

  var PROBE = {
    liveness: ["Liveness probe", "When it fails, Kubernetes restarts the container."],
    readiness: ["Readiness probe", "When it fails, the pod gets no traffic from its Services."],
    startup: ["Startup probe", "Until it passes, the other probes wait. When it gives up, Kubernetes restarts the container."]
  };

  var CAPACITY = {
    spot: ["Spot", "A cheaper node the cloud can take back at short notice."],
    "on-demand": ["On-demand", "A regular node the cloud does not take back."]
  };

  /* fold shows or hides a section and returns its body, emptied. */
  function fold(root, name, show) {
    var f = KW.$('[data-fold="' + name + '"]', root);
    var box = KW.$('[data-' + name + ']', root);
    box.textContent = "";
    f.hidden = !show;
    return box;
  }

  function cap(box, text, cut, one, many) {
    var t = text || "";
    if (cut) t += (t ? " " : "") + KW.plural(cut, one, many) + " not shown.";
    if (t) box.appendChild(KW.el("p", "ev-cap", t));
  }

  /* facts adds a small labelled list: [label, value, tip] rows. */
  function facts(parent, rows) {
    var dl = KW.el("dl", "in-facts");
    rows.forEach(function (r) {
      if (r[1] === "" || r[1] === null || r[1] === undefined) return;
      var dt = dl.appendChild(KW.el("dt", "", r[0]));
      if (r[2]) dt.title = r[2];
      var dd = KW.el("dd", r[3] || "");
      if (r[1].nodeType) dd.appendChild(r[1]);
      else dd.textContent = r[1];
      dl.appendChild(dd);
    });
    parent.appendChild(dl);
    return dl;
  }

  function lastSeen(iso, count) {
    var s = KW.el("span");
    if (count > 1) s.appendChild(document.createTextNode("seen " + count + " times · "));
    s.appendChild(KW.timeEl(iso, "last "));
    return s;
  }

  function waiting(root, inc) {
    var w = inc.waiting;
    var box = fold(root, "waiting", !!w);
    if (!w) return;
    box.appendChild(KW.el("p", "", w.pods === 1 ? "1 pod is waiting for a node to run on." :
      w.pods + " pods are waiting for a node to run on."));
    if (w.reasons && w.reasons.length) {
      box.appendChild(KW.el("p", "in-lead", w.nodes ?
        "The scheduler looked at " + KW.plural(w.nodes, "node", "nodes") + " and ruled them out:" :
        "The scheduler ruled nodes out because:"));
      var ul = KW.el("ul", "in-list");
      w.reasons.forEach(function (r) {
        var li = KW.el("li", "", r.text);
        li.title = "The scheduler said: " + r.reason;
        li.appendChild(KW.el("span", "in-n", " · " + (w.nodes ? r.nodes + " of " + w.nodes :
          KW.plural(r.nodes, "node", "nodes"))));
        ul.appendChild(li);
      });
      box.appendChild(ul);
    }
    box.appendChild(KW.el("h3", "fold-h", "What the scheduler said"));
    box.appendChild(KW.el("p", "in-raw mono", w.message));
    var meta = KW.el("small", "cx-meta", "About pod " + w.namespace + "/" + w.pod);
    if (KW.valid(w.at)) {
      meta.appendChild(document.createTextNode(" · "));
      meta.appendChild(KW.timeEl(w.at));
    }
    box.appendChild(meta);
    cap(box, "", KW.cutCount(inc, "waitReasons"), "more reason", "more reasons");
  }

  function timing(p) {
    var parts = [];
    if (p.periodSeconds) parts.push("every " + p.periodSeconds + " s");
    if (p.timeoutSeconds) parts.push("waits " + p.timeoutSeconds + " s for an answer");
    if (p.failureThreshold) {
      parts.push("fails after " + KW.plural(p.failureThreshold, "miss", "misses") + " in a row");
    }
    if (p.delaySeconds) parts.push("starts " + p.delaySeconds + " s after the container");
    return parts.join(", ");
  }

  function probes(root, inc) {
    var list = inc.probes || [];
    var box = fold(root, "probes", list.length > 0);
    if (!list.length) return;
    list.forEach(function (p) {
      var item = KW.el("div", "in-item");
      var kind = PROBE[p.probe] || [p.probe + " probe", ""];
      var h = item.appendChild(KW.el("p", "in-head"));
      var b = h.appendChild(KW.el("b", "", kind[0]));
      if (kind[1]) b.title = kind[1];
      h.appendChild(document.createTextNode(" of container "));
      h.appendChild(KW.el("span", "mono", p.container));
      h.appendChild(document.createTextNode(" in pod "));
      h.appendChild(KW.el("span", "mono", p.pod));
      facts(item, [
        ["Checks", p.check || "", "What the probe asks the container. An exec probe shows only the program, never its arguments.", "mono"],
        ["Timing", timing(p), "How often it checks, how long it waits and how many misses fail it."],
        ["Last failure", p.lastMessage || "", "", "in-raw"],
        ["Seen", KW.valid(p.lastAt) ? lastSeen(p.lastAt, p.count) : ""]
      ]);
      box.appendChild(item);
    });
    cap(box, "", KW.cutCount(inc, "probes"), "more probe", "more probes");
  }

  function warnings(root, inc) {
    var list = inc.warnings || [];
    var box = fold(root, "warnings", list.length > 0);
    if (!list.length) return;
    KW.$("[data-warnings-n]", root).textContent = "(" + list.length + ")";
    var ul = box.appendChild(KW.el("ul", "cx-list"));
    list.forEach(function (w) {
      var li = KW.el("li", "in-warn");
      var line = li.appendChild(KW.el("p", "cx-line"));
      line.appendChild(KW.el("b", "", w.reason));
      line.appendChild(document.createTextNode(": " + w.message));
      if (KW.isNew(w.lastSeen)) {
        li.className += " is-new";
        line.appendChild(KW.newMark());
      }
      var meta = li.appendChild(KW.el("small", "cx-meta"));
      meta.appendChild(lastSeen(w.lastSeen, w.count));
      meta.appendChild(document.createTextNode(" · on " + w.object +
        (w.objects > 1 ? " and " + KW.plural(w.objects - 1, "more", "more") : "")));
      ul.appendChild(li);
    });
    cap(box, "Kubernetes Warning events about this issue's objects, the same message counted once. Secrets are removed.",
      KW.cutCount(inc, "warnings"), "more warning", "more warnings");
  }

  function trafficRow(r) {
    var li = KW.el("li", "in-row");
    var bad = r.total === 0 || r.ready < r.total;
    var dot = li.appendChild(KW.el("span", "ao-dot " + (r.ready === 0 ? "s-failing" : bad ? "s-cause" : "s-recovered")));
    dot.setAttribute("aria-hidden", "true");
    var p = li.appendChild(KW.el("p"));
    p.appendChild(document.createTextNode((r.kind === "ingress" ? "Ingress " : "Service ")));
    p.appendChild(KW.el("span", "mono", r.namespace + "/" + r.name));
    if (r.backends && r.backends.length) {
      p.appendChild(document.createTextNode(" → " + r.backends.join(", ")));
    }
    p.appendChild(document.createTextNode(": "));
    p.appendChild(KW.el("b", bad ? "x-bad" : "", r.total === 0 ? "no endpoints" :
      r.ready + " of " + KW.plural(r.total, "endpoint", "endpoints") + " ready"));
    return li;
  }

  function traffic(root, inc) {
    var list = inc.traffic || [];
    var box = fold(root, "traffic", list.length > 0);
    if (!list.length) return;
    var ul = box.appendChild(KW.el("ul", "in-rows"));
    list.forEach(function (r) { ul.appendChild(trafficRow(r)); });
    cap(box, "Endpoints are the pods a Service sends traffic to; only ready ones get it. Counted now.",
      KW.cutCount(inc, "traffic"), "more", "more");
  }

  function nodeStatus(n) {
    var parts = [];
    if (n.ready === false) parts.push("Not ready");
    else if (n.ready === true) parts.push("Ready");
    if (n.cordoned) parts.push("cordoned (no new pods)");
    if (n.deleting) parts.push("being deleted");
    return parts.join(", ");
  }

  function conditions(n) {
    if (!n.conditions || !n.conditions.length) return "";
    var ul = KW.el("ul", "in-list in-conds");
    n.conditions.forEach(function (c) {
      var li = KW.el("li");
      li.appendChild(KW.el("b", "", c.type + " is " + c.status));
      var rest = [c.reason, c.message].filter(Boolean).join(": ");
      if (rest) li.appendChild(document.createTextNode(" · " + rest));
      if (KW.valid(c.since)) li.title = "Since " + KW.full(c.since);
      ul.appendChild(li);
    });
    return ul;
  }

  function nodes(root, inc) {
    var list = inc.nodes || [];
    var box = fold(root, "node", list.length > 0);
    if (!list.length) return;
    KW.$("[data-node-h]", root).textContent = list.length > 1 ? "Nodes" : "Node";
    list.forEach(function (n) {
      var item = box.appendChild(KW.el("div", "in-item"));
      var h = item.appendChild(KW.el("p", "in-head"));
      h.appendChild(KW.el("b", "mono", n.name));
      if (n.root) h.appendChild(KW.chip("c-notify", "This issue's node"));
      var capType = CAPACITY[n.capacityType];
      var capEl = "";
      if (capType) {
        capEl = KW.el("span", "", capType[0]);
        capEl.title = capType[1];
      }
      facts(item, [
        ["Status", nodeStatus(n), "", n.ready === false ? "x-bad" : ""],
        ["Problems", conditions(n), "Node conditions that are not healthy now."],
        ["Zone", n.zone || ""],
        ["Capacity", capEl, "Spot nodes can be taken back by the cloud."],
        ["Machine", [n.instanceType, n.pool ? "pool " + n.pool : ""].filter(Boolean).join(" · ")],
        ["Age", KW.valid(n.created) ? KW.lasted(n.created, new Date(KW.now()).toISOString()) : ""],
        ["This issue's pods", n.root ? "" : String(n.pods)],
        ["Other pods failing", String(n.otherFailing), "Pods of other workloads on this node that are not ready now.",
          n.otherFailing ? "x-warn" : ""]
      ]);
    });
    cap(box, "", KW.cutCount(inc, "nodes"), "more node", "more nodes");
  }

  KW.insight = function (root, inc) {
    waiting(root, inc);
    probes(root, inc);
    warnings(root, inc);
    traffic(root, inc);
    nodes(root, inc);
  };
})();
