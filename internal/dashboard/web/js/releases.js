/* kwatch dashboard: the Releases page (/releases): every rollout of a
   Deployment, StatefulSet or DaemonSet in the last 7 days (or 24 hours,
   ?since=24h), newest first, from /api/releases, with its verdict: still
   rolling, watched, healthy, regressed, failed while the previous version
   still serves, down, replaced by a newer release, or unknown (seen after
   the fact). Each row says what changed and how the rollout went; one
   that went wrong says how to undo it and links to its issue. Filters:
   namespace and verdict, kept in the address. A healthy release sends no
   message: this page is where it shows. */
(function () {
  "use strict";
  var KW = window.KW;
  var VERDICTS = {
    rolling: ["Rolling out", "c-open", "The new pods are still replacing the old ones."],
    watching: ["Watching", "c-open", "Rolled out; kwatch watches it for 15 minutes."],
    healthy: ["Healthy", "c-resolved", "Rolled out and nothing broke while kwatch watched it."],
    regressed: ["Regressed", "c-notify", "The new version restarts far more than the previous one."],
    failed: ["Failed", "c-notify", "The new version fails; the workload still serves."],
    down: ["Down", "c-page", "The workload has no ready replica."],
    replaced: ["Replaced", "c-quiet", "A newer release came before a verdict."],
    unknown: ["Not watched", "c-quiet", "kwatch saw this release only after it happened."]
  };
  /* FILTERS offers every verdict the API filters by (releaseVerdicts in
     releases_api.go), so a link with any of them shows what it filters. */
  var FILTERS = [["", "Any verdict"], ["bad", "Went wrong"]].concat(
    Object.keys(VERDICTS).map(function (k) { return [k, VERDICTS[k][0]]; }));
  var WINDOWS = { "24h": "the last 24 hours", "7d": "the last 7 days" };

  /* short is an image without its registry: "shop/api:2.4". */
  function short(image) {
    var parts = (image || "").split("/");
    return parts.length > 2 ? parts.slice(-2).join("/") : image || "";
  }

  function what(r) {
    var out = (r.images || []).map(function (i) {
      return i.before ? short(i.before) + " → " + short(i.after) : "added " + short(i.after);
    });
    if (r.changed && r.changed.length) out.push(r.changed.join(", "));
    return out.join(" · ") || "pod template";
  }

  function how(r) {
    if (r.stalled) {
      return r.deadline ? "Stalled: it missed its progress deadline." :
        "Stalled: still not rolled out after 15 minutes.";
    }
    if (typeof r.rolloutSeconds === "number") {
      return "Rolled out in " + KW.duration(r.rolloutSeconds) + ".";
    }
    return r.desired ? KW.num(r.ready) + " of " + KW.num(r.desired) + " ready." : "";
  }

  function serving(r) {
    if (r.verdict === "down") return "Nothing is ready.";
    if (r.oldServing) return "The previous version still serves " + KW.num(r.ready) + " of " + KW.num(r.desired) + ".";
    return "";
  }

  function who(r) {
    var bits = [];
    if (r.app) bits.push(r.app);
    else if (r.actor) bits.push(r.actor);
    if (r.cause) bits.push("“" + r.cause + "”");
    return bits.join(" · ");
  }

  function detail(r, box) {
    var d = r.detail;
    if (!d) return;
    var parts = [];
    if (typeof d.restartsBefore === "number" || typeof d.restartsAfter === "number") {
      parts.push("Restarts per hour: " + (typeof d.restartsBefore === "number" ? KW.num(d.restartsBefore) : "?") +
        " before, " + (typeof d.restartsAfter === "number" ? KW.num(d.restartsAfter) : "?") + " after.");
    }
    if (typeof d.readyBefore === "number") parts.push("Usually ready in " + KW.duration(d.readyBefore) + ".");
    if (parts.length) box.appendChild(KW.el("p", "rel-line", parts.join(" ")));
    if (d.errorLine) box.appendChild(KW.el("p", "rel-error mono", d.errorLine));
  }

  function row(r) {
    var li = KW.el("li", "rel-row");
    var top = li.appendChild(KW.el("div", "rel-top"));
    top.appendChild(KW.hm(r.at)).className = "rel-at";
    var v = VERDICTS[r.verdict] || [r.verdict, "c-quiet", ""];
    top.appendChild(KW.chip(v[1], v[0], v[2]));
    var name = top.appendChild(KW.el("span", "rel-name mono", r.kind + "/" + r.name));
    name.title = r.namespace + "/" + r.name;
    top.appendChild(KW.el("span", "rel-ns mono", r.namespace));
    var body = li.appendChild(KW.el("div", "rel-body"));
    body.appendChild(KW.el("p", "rel-what mono", what(r)));
    var line = [how(r), serving(r)].filter(Boolean).join(" ");
    if (line) body.appendChild(KW.el("p", "rel-line", line));
    var w = who(r);
    if (w) body.appendChild(KW.el("p", "rel-who", w));
    detail(r, body);
    if (["regressed", "failed", "down"].indexOf(r.verdict) >= 0) {
      if (r.incident) {
        var p = body.appendChild(KW.el("p", "rel-line"));
        var a = p.appendChild(KW.el("a", "ulink", "Open its issue"));
        a.href = "/issues/" + encodeURIComponent(r.incident);
        a.dataset.link = "";
      }
      if (r.undo && r.undo.text) {
        body.appendChild(KW.el("p", "rel-line rel-undo", r.undo.text));
        if (r.undo.command) body.appendChild(KW.cmd(r.undo.command));
      }
    }
    return li;
  }

  function picker(label, name, options, value) {
    var l = KW.el("label", "f rel-f");
    l.appendChild(KW.el("span", "sr-only", label));
    var s = l.appendChild(KW.el("select"));
    s.name = name;
    s.setAttribute("aria-label", label);
    options.forEach(function (o) {
      var opt = s.appendChild(KW.el("option", "", o[1]));
      opt.value = o[0];
      if (o[0] === value) opt.selected = true;
    });
    return { label: l, select: s };
  }

  KW.releasesView = function (main) {
    var query = new URLSearchParams(location.search);
    var since = query.get("since") === "24h" ? "24h" : "7d";
    var namespace = query.get("namespace") || "";
    var verdict = query.get("verdict") || "";
    var etag = "", seq = 0, spaces = [];
    var pg = KW.pageView(main, { title: "Releases", tab: "Releases · kwatch", read: load,
      sub: "Every rollout and how it went. A healthy one sends no message; it shows here." });
    var bar = pg.body.appendChild(KW.el("div", "rel-bar"));
    var sw = bar.appendChild(KW.el("div", "tz rel-sw"));
    sw.setAttribute("role", "group");
    sw.setAttribute("aria-label", "Show releases from");
    var buttons = ["24h", "7d"].map(function (w) {
      var b = sw.appendChild(KW.el("button", "tz-b", w === "24h" ? "Last 24 hours" : "Last 7 days"));
      b.type = "button";
      b.dataset.relSince = w;
      return b;
    });
    var nsPick = picker("Namespace", "rel-namespace", [["", "All namespaces"]], namespace);
    bar.appendChild(nsPick.label);
    var vPick = picker("Verdict", "rel-verdict", FILTERS, verdict);
    bar.appendChild(vPick.label);
    var sum = pg.body.appendChild(KW.el("p", "rel-sum"));
    var list = pg.body.appendChild(KW.el("ul", "rel-list"));
    pg.body.hidden = true;

    function address() {
      var q = new URLSearchParams();
      if (since === "24h") q.set("since", "24h");
      if (namespace) q.set("namespace", namespace);
      if (verdict) q.set("verdict", verdict);
      var s = q.toString();
      return "/releases" + (s ? "?" + s : "");
    }

    function mark() {
      buttons.forEach(function (b) {
        b.setAttribute("aria-pressed", b.dataset.relSince === since ? "true" : "false");
      });
    }

    function fillSpaces(names) {
      if (names.join(",") === spaces.join(",")) return;
      spaces = names.slice();
      var s = nsPick.select;
      while (s.options.length > 1) s.remove(1);
      names.forEach(function (n) {
        var opt = s.appendChild(KW.el("option", "", n));
        opt.value = n;
      });
      // Through the setter, so the drawn picker shows it too.
      s.value = names.indexOf(namespace) >= 0 ? namespace : "";
    }

    function draw(data) {
      var items = data.releases || [];
      fillSpaces(data.namespaces || []);
      list.textContent = "";
      items.forEach(function (r) { list.appendChild(row(r)); });
      list.hidden = !items.length;
      sum.classList.toggle("is-empty", !data.total);
      sum.textContent = !data.total ? "No releases in " + WINDOWS[since] + "." :
        KW.plural(data.total, "release", "releases") + " in " + WINDOWS[since] +
        (data.bad ? ", " + KW.num(data.bad) + " went wrong." : ", none went wrong.") +
        (items.length !== data.total ? " Showing " + KW.num(items.length) + "." : "");
    }

    function load() {
      var mine = ++seq;
      var q = "?since=" + since + (namespace ? "&namespace=" + encodeURIComponent(namespace) : "") +
        (verdict ? "&verdict=" + encodeURIComponent(verdict) : "");
      return KW.api("/api/releases" + q, { etag: etag }).then(function (res) {
        if (mine !== seq) return null;
        if (res.status === 200 && res.data) {
          etag = res.etag;
          draw(res.data);
        }
        return res;
      });
    }

    function reload() {
      etag = "";
      mark();
      history.replaceState(null, "", address());
      KW.rendered = location.pathname + location.search;
      pg.now();
    }

    buttons.forEach(function (b) {
      b.addEventListener("click", function () { since = b.dataset.relSince; reload(); });
    });
    nsPick.select.addEventListener("change", function () { namespace = nsPick.select.value; reload(); });
    vPick.select.addEventListener("change", function () { verdict = vPick.select.value; reload(); });
    mark();
    return { stop: pg.stop };
  };
})();
