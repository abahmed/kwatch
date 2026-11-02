/* kwatch dashboard: the filter bar of the Issues table. The status
   switch (Active, Resolved, All), the search box, the multi-select
   pickers (namespace, kind, state, acknowledged, stopped tracking: dropdown.js), the
   start time and "resolved within" pickers and the severity chips with
   their counts. It puts a view (listquery.js)
   into the controls, reads it back, and paints the counts the API
   returns (facets). The chips of the filters in use are drawn by
   filterchips.js. Under 700px wide the filters fold behind one
   "Filters (2)" button. issues.js owns it. */
(function () {
  "use strict";
  var KW = window.KW;
  var LQ = function () { return KW.listQuery; };
  var STATUS_WORDS = { active: "Active", resolved: "Resolved", all: "All" };
  var ACKED = { "true": "Yes", "false": "No" };
  var STATE_ORDER = ["open", "settling", "flapping", "recovering", "resolved"];

  var n = function (x) { return KW.num(x); };

  /* PICKERS: the multi-select filters, by view key. */
  var PICKERS = {
    namespace: { name: "Namespace", all: "All namespaces", mono: true,
      search: true },
    kind: { name: "Kind", all: "All kinds", noun: "kind of object" },
    state: { name: "State", all: "Any state" },
    acked: { name: "Acknowledged", all: "Acknowledged or not",
      noun: "value" },
    stopped: { name: "Stopped tracking", all: "Stopped tracking or not",
      noun: "value" }
  };

  /* issueFilters wires box. onChange(next, typing) gets the view the
     controls show; typing is true while the search text changes. */
  KW.issueFilters = function (box, onChange) {
    var $ = function (sel) { return KW.$(sel, box); };
    var q = $('[data-f="q"]'), since = $('[data-f="since"]');
    var age = $('[data-f="age"]'), sortSel = $('[data-f="sort"]');
    var dirBtn = $("[data-f-dir]");
    var statusBtns = KW.$$("[data-status]", box);
    var chips = KW.$$(".fchip", box);
    var f = LQ().blank();
    var debounce = 0, facets = {}, modes = {};
    var chipsRow = KW.filterChips($("[data-factive]"), function (next) {
      change(next);
    });
    var page = box.closest(".content"), toggle = $("[data-ftoggle]");
    // The search tips show three; "More" shows the rest.
    var moreTips = $("[data-shint-more]"), restTips = $("[data-shint-rest]");
    moreTips.addEventListener("click", function () {
      restTips.hidden = !restTips.hidden;
      moreTips.textContent = restTips.hidden ? "More" : "Fewer";
      moreTips.setAttribute("aria-expanded", restTips.hidden ? "false" : "true");
    });
    toggle.addEventListener("click", function () {
      var open = !page.classList.contains("f-open");
      page.classList.toggle("f-open", open);
      toggle.setAttribute("aria-expanded", open ? "true" : "false");
    });

    /* countUsed: how many filters narrow the list, for the button
       that shows the filters under 700px. */
    function countUsed(v) {
      var c = (v.q ? 1 : 0) + (v.like ? 1 : 0) + (v.age ? 1 : 0) +
        (v.since && v.status !== "active" ? 1 : 0);
      LQ().MULTI.forEach(function (k) { c += v[k].length; });
      return c;
    }

    var pick = {};
    Object.keys(PICKERS).forEach(function (key) {
      var cfg = Object.assign({}, PICKERS[key], { onChange: function (values) {
        var patch = {};
        patch[key] = values;
        change(patch);
      } });
      pick[key] = KW.multiSelect(cfg);
      $('[data-ms="' + key + '"]').appendChild(pick[key].el);
    });

    function pressed(list, attr, value) {
      list.forEach(function (b) {
        b.setAttribute("aria-pressed", b.dataset[attr] === value ? "true" : "false");
      });
    }

    /* counted: the values of a facet, most issues first (then A to Z),
       and the values picked, which stay listed even at no issues. */
    function counted(facet, picked) {
      var by = Object.assign({}, facet || {});
      picked.forEach(function (v) { if (by[v] === undefined) by[v] = 0; });
      return Object.keys(by).sort(function (a, b) {
        return by[b] - by[a] || (a < b ? -1 : a > b ? 1 : 0);
      }).map(function (v) { return { value: v, count: by[v] }; });
    }

    /* items: what each picker offers, from the facets of the last
       response. */
    function items() {
      pick.namespace.items(counted(facets.namespace, f.namespace).map(function (x) {
        return { value: x.value, label: x.value, count: x.count };
      }));
      var kinds = kindLabels();
      pick.kind.items(counted(lowerKeys(facets.kind), f.kind).map(function (x) {
        return { value: x.value, label: kinds[x.value] || x.value, count: x.count };
      }));
      var st = facets.state || {};
      pick.state.items(STATE_ORDER.filter(function (s) {
        return s !== "resolved" || f.status === "all";
      }).map(function (s) {
        return { value: s, label: KW.STATE[s].label, count: st[s] || 0,
          title: KW.STATE[s].title };
      }));
      var ack = facets.acked || {};
      pick.acked.items(["true", "false"].map(function (v) {
        return { value: v, label: ACKED[v], count: ack[v] || 0 };
      }));
      var stp = facets.stopped || {};
      pick.stopped.items(["true", "false"].map(function (v) {
        return { value: v, label: ACKED[v], count: stp[v] || 0 };
      }));
    }

    function lowerKeys(by) {
      var out = {};
      Object.keys(by || {}).forEach(function (k) { out[k.toLowerCase()] = by[k]; });
      return out;
    }

    /* show puts the view v into the controls. */
    function show(v) {
      f = v;
      // While the reader types, the box keeps what they typed (spaces
      // included): the view only has it trimmed.
      if (document.activeElement !== q || q.value.trim() !== v.q) q.value = v.q;
      var used = countUsed(v);
      toggle.textContent = used ? "Filters (" + used + ")" : "Filters";
      since.value = v.since;
      age.value = v.age;
      sortSel.value = v.sort;
      dirBtn.hidden = !v.sort;
      var desc = v.dir ? v.dir === "desc" : !!KW.issueTable.DESCENDS[v.sort];
      dirBtn.textContent = desc ? "↓ High to low" : "↑ Low to high";
      pressed(statusBtns, "status", v.status);
      chips.forEach(function (b) {
        b.setAttribute("aria-pressed", v.severity.indexOf(b.dataset.sev) >= 0 ? "true" : "false");
      });
      Object.keys(pick).forEach(function (k) { pick[k].set(v[k]); });
      pick.state.el.parentNode.hidden = v.status === "resolved";
      // Only resolved issues stop being tracked.
      pick.stopped.el.parentNode.hidden = v.status === "active" && !v.stopped.length;
      since.closest(".f").hidden = v.status === "active";
      items();
      chipsRow.show(v, kindLabels());
    }

    function kindLabels() {
      var out = {};
      Object.keys(facets.kind || {}).forEach(function (k) { out[k.toLowerCase()] = k; });
      return out;
    }

    /* change hands a copy of f with patch applied to onChange. A change
       of filters starts again at the first page. */
    function change(patch, typing) {
      var next = LQ().copy(f, patch);
      if (!("page" in patch)) next.page = 1;
      if (next.status === "resolved") next.state = [];
      if (next.status === "active") {
        next.state = next.state.filter(function (s) { return s !== "resolved"; });
      }
      show(next);
      onChange(next, !!typing);
    }

    q.addEventListener("input", function () {
      window.clearTimeout(debounce);
      debounce = window.setTimeout(function () { change({ q: q.value.trim() }, true); }, 300);
    });
    q.addEventListener("keydown", function (e) {
      if (e.key !== "Enter") return;
      window.clearTimeout(debounce);
      change({ q: q.value.trim() }, false);
    });
    [[since, "since"], [age, "age"]].forEach(function (p) {
      p[0].addEventListener("change", function () {
        var patch = {};
        patch[p[1]] = p[0].value;
        change(patch);
      });
    });
    sortSel.addEventListener("change", function () { change({ sort: sortSel.value, dir: "" }); });
    dirBtn.addEventListener("click", function () {
      var desc = f.dir ? f.dir === "desc" : !!KW.issueTable.DESCENDS[f.sort];
      change({ dir: desc ? "asc" : "desc" });
    });
    statusBtns.forEach(function (b) {
      b.addEventListener("click", function () { change({ status: b.dataset.status }); });
    });
    chips.forEach(function (b) {
      b.addEventListener("click", function () {
        var sev = f.severity.slice(), at = sev.indexOf(b.dataset.sev);
        if (at >= 0) sev.splice(at, 1); else sev.push(b.dataset.sev);
        change({ severity: sev });
      });
    });

    /* paint shows the counts of a list response: per status, per
       severity and in the pickers, each counted without its own
       filter, so a value says what picking it shows. */
    function paint(data) {
      facets = data.facets || {};
      var byStatus = facets.state || {};
      var active = (byStatus.open || 0) + (byStatus.settling || 0) +
        (byStatus.flapping || 0) + (byStatus.recovering || 0);
      var counts = { active: active, resolved: byStatus.resolved || 0,
        all: active + (byStatus.resolved || 0) };
      // The counts follow the search and filters, so while any is on they
      // would read 0 for issues that exist: they are left out then.
      var narrowed = LQ().used(f);
      statusBtns.forEach(function (b) {
        var k = b.dataset.status;
        KW.$("[data-n]", b).hidden = narrowed;
        KW.$("[data-n]", b).textContent = n(counts[k]);
        if (narrowed) b.setAttribute("aria-label", STATUS_WORDS[k]);
        else b.setAttribute("aria-label", STATUS_WORDS[k] + ", " + KW.plural(counts[k], "issue", "issues"));
      });
      chips.forEach(function (b) {
        var c = (facets.severity || {})[b.dataset.sev] || 0;
        KW.$("[data-n]", b).textContent = n(c);
        var name = KW.SEVERITY[b.dataset.sev].label;
        b.title = KW.SEVERITY[b.dataset.sev].title + " Pick several to see them together.";
        b.setAttribute("aria-label", name + ", " + KW.plural(c, "issue", "issues"));
        b.hidden = c === 0 && f.severity.indexOf(b.dataset.sev) < 0;
      });
      $(".fchips").hidden = chips.every(function (b) { return b.hidden; });
      items();
      modes = data.modeLabels || {};
      chipsRow.show(f, kindLabels(), data.total, modes);
    }

    return { show: show, paint: paint,
      stop: function () { window.clearTimeout(debounce); } };
  };
})();
