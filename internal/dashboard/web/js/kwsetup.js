/* kwatch dashboard: the setup checklist on the kwatch health page
   (#setup): five things a cluster needs for kwatch to do its job, each
   done or not, with one line on how to do what is missing: a cluster
   name, the heartbeat, at least one alert channel, Secrets watched and
   complete permissions. From /api/status (its kwatch part) and
   /api/coverage. Words only; nothing here changes the cluster. */
(function () {
  "use strict";
  var KW = window.KW;
  var HB_OK = { ok: true, waiting: true };

  /* secretsState: true when kwatch watches Secrets, false when it
     cannot or does not, null when the coverage is not known yet. */
  function secretsState(cov) {
    var k = cov && cov.kinds;
    if (!k) return null;
    function has(list) {
      return (list || []).some(function (g) { return String(g.kind).toLowerCase() === "secrets"; });
    }
    if (has(k.unreadable) || has(k.notWatched)) return false;
    return has(k.typed) ? true : null;
  }

  /* items: [label, done (true, false or null for not known yet), the
     line under it]. */
  function items(s, k, cov) {
    var name = k && k.clusterName, hb = (k && k.heartbeat) || {};
    var channels = ((k && k.delivery) || []).length || ((s && s.perChannelHealth) || []).length;
    var t = (k && k.coverage) || {};
    var blocked = (t.unreadable || 0) > 0 || !!(s && s.self && s.self.podLogsForbidden);
    var secrets = secretsState(cov);
    return [
      ["Cluster name", name ? !!name.name : null, name && name.name ?
        name.name + (name.source ? " (" + name.source + ")" : "") :
        "Set app.clusterName in the chart's values, so alerts and this dashboard say which cluster they are about."],
      ["Heartbeat", hb.state ? !!HB_OK[hb.state] : null, HB_OK[hb.state] ?
        KW.TERMS.heartbeat[hb.state] || "Pinging" :
        KW.TERMS.heartbeat[hb.state] || "Set heartbeatMonitor.url, so you learn when kwatch itself stops."],
      ["At least one alert channel", s || k ? channels > 0 : null, channels > 0 ?
        KW.plural(channels, "channel", "channels") + " set up." :
        "Add Slack, a pager or another channel under notifications in the chart's values."],
      ["Secrets watched", secrets, secrets ? "kwatch sees Secrets (only a hash of their data)." :
        "Let kwatch list and watch Secrets, so a missing or changed Secret can be named as a cause."],
      ["Permissions complete", t.total || s ? !blocked : null, !blocked ? "kwatch can read everything it watches." :
        "kwatch can't read some kinds or pod logs: see what it can't read below, with the rules to add."]
    ];
  }

  /* KW.setupCard: the checklist as a card; "3 of 5 done" in its head. */
  KW.setupCard = function (s, k, cov) {
    var list = items(s, k, cov);
    var done = list.filter(function (x) { return x[1] === true; }).length;
    var c = KW.viz.card("Setup checklist", KW.num(done) + " of " + list.length + " done.");
    c.id = "setup";
    c.classList.add("kp-setup");
    var ul = c.appendChild(KW.el("ul", "kp-check"));
    list.forEach(function (x) {
      var li = ul.appendChild(KW.el("li", x[1] === true ? "is-done" : x[1] === false ? "is-todo" : "is-unknown"));
      li.dataset.check = x[0];
      var mark = li.appendChild(KW.el("span", "kp-check-m", x[1] === true ? "✓" : x[1] === false ? "!" : "…"));
      mark.setAttribute("aria-hidden", "true");
      var body = li.appendChild(KW.el("span", "kp-check-b"));
      body.appendChild(KW.el("b", "", x[0]));
      body.appendChild(KW.el("span", "sr-only", x[1] === true ? " (done)" : x[1] === false ? " (not done)" : " (not known yet)"));
      body.appendChild(KW.el("span", "kp-check-t", x[2]));
    });
    return c;
  };
})();
