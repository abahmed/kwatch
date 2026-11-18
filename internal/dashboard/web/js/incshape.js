/* kwatch dashboard: the shape of an issue, in "What's wrong?" on the
   Summary tab. Whether users are affected comes first
   (summary.users); after the sentence, what ties the members of a
   grouped issue together with the members by group (summary.grouped),
   and what the failing replicas share that the healthy ones do not
   (summary.replicas). Each is drawn only when the server sent it. */
(function () {
  "use strict";
  var KW = window.KW;

  function summary(inc) { return inc.summary || {}; }

  function objName(o) {
    return KW.obj(o.kind, o.namespace, o.name);
  }

  /* KW.usersLine: "Users are affected: route … has 0 of 3 ready
     endpoints." at the top of "What's wrong?". */
  KW.usersLine = function (dd, inc) {
    var u = summary(inc).users;
    if (!u || !u.text) return;
    var p = dd.appendChild(KW.el("p", "sh-users", u.text));
    p.setAttribute("data-users", u.state || "");
    if (u.state === "affected") p.classList.add("sh-hit");
  };

  function groupItem(ul, g) {
    var li = ul.appendChild(KW.el("li"));
    li.appendChild(KW.wrapName(objName(g.owner)));
    var names = (g.members || []).map(objName);
    if (g.more) names.push(KW.plural(g.more, "more", "more"));
    if (names.length) li.appendChild(KW.el("span", "sh-members", " · " + names.join(", ")));
  }

  /* KW.groupedLine: "Grouped because …" and, folded, the members by
     the workload or object they belong to. */
  KW.groupedLine = function (dd, inc) {
    var g = summary(inc).grouped;
    if (!g || !g.text) return;
    var box = dd.appendChild(KW.el("div", "sh-grouped"));
    box.setAttribute("data-grouped", g.basis || "");
    box.appendChild(KW.el("p", "sh-why", g.text));
    var groups = g.groups || [];
    if (!groups.length) return;
    var det = box.appendChild(KW.el("details", "sh-groups"));
    var n = groups.length + (g.more || 0);
    det.appendChild(KW.el("summary", "", "Members by group (" + KW.num(n) + ")"));
    var ul = det.appendChild(KW.el("ul", "sh-list"));
    groups.forEach(function (one) { groupItem(ul, one); });
    if (g.more) ul.appendChild(KW.el("li", "sh-more", "+" + KW.plural(g.more, "more group", "more groups") + " not shown"));
  };

  /* KW.replicaLines: "Only the 3 pods on arm64 nodes fail; the 4 pods
     on amd64 nodes are healthy." An observation, not the cause. */
  KW.replicaLines = function (dd, inc) {
    var lines = summary(inc).replicas || [];
    lines.forEach(function (line) {
      dd.appendChild(KW.el("p", "sh-split", line)).setAttribute("data-replicas", "");
    });
  };
})();
