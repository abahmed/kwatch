/* kwatch dashboard: the filters in use, under the filter bar of the
   Issues table, as chips that each remove one value ("Namespace:
   team-1 ×", "Similar to: <title> ×"), with how many issues match and
   "Clear all". Each filter shows once: the values of the pickers here
   (in the pickers' own words), while the search box, the severity
   chips and the start time picker show their own. issuesfilters.js
   owns it. */
(function () {
  "use strict";
  var KW = window.KW;
  var LQ = function () { return KW.listQuery; };
  var NAMES = { namespace: "Namespace", kind: "Kind", state: "State",
    acked: "Acknowledged", stopped: "Stopped tracking", node: "Node",
    mode: "Failure" };
  var ACKED = { "true": "Yes", "false": "No" };

  /* word: a filter value in words. modes are the server's labels of
     the failure types ("CPUHigh" is "CPU high"). */
  function word(key, v, kinds, modes) {
    if (key === "state") return KW.STATE[v] ? KW.STATE[v].label : v;
    if (key === "acked" || key === "stopped") return ACKED[v] || v;
    if (key === "kind") return kinds[v] || v;
    if (key === "mode") return modes[v] || v;
    return v;
  }

  /* filterChips draws into box. onChange(next) gets the view without
     the value removed. */
  KW.filterChips = function (box, onChange) {
    var f = null, kindsShown = {}, titles = {}, modesShown = {}, gone = {};
    box.setAttribute("aria-label", "Filters in use");

    var made = 0;
    function chip(name, value, patch, mono) {
      var b = KW.el("button", "fpill");
      b.type = "button";
      b.dataset.fk = "chip:" + (made++);
      b.appendChild(KW.el("span", "fpill-k", name + ":"));
      b.appendChild(KW.el("span", "fpill-v" + (mono ? " mono" : ""), value));
      b.appendChild(KW.icon("x"));
      b.title = "Remove " + name + ": " + value;
      b.setAttribute("aria-label", "Remove filter " + name + ": " + value);
      b.addEventListener("click", function () {
        onChange(LQ().copy(f, Object.assign(patch(), { page: 1 })));
      });
      return b;
    }

    function without(key, v) {
      return function () {
        var o = {};
        o[key] = f[key].filter(function (x) { return x !== v; });
        return o;
      };
    }

    /* likeTitle: the title of the issue like= names, read once; its ID
       until then. An issue that is gone (404) is remembered for the
       session, so a redraw never asks again. */
    function likeTitle(id) {
      if (gone[id]) return "an issue no longer kept";
      if (titles[id] === undefined) {
        titles[id] = id;
        KW.api("/api/incidents/" + encodeURIComponent(id)).then(function (res) {
          if (res.status === 404) {
            gone[id] = true;
          } else if (res.status !== 200 || !res.data || !res.data.title) {
            // Forget the placeholder so a later show() asks again.
            delete titles[id];
            return;
          } else {
            titles[id] = res.data.title;
          }
          if (f && f.like === id) show(f, kindsShown);
        });
      }
      return titles[id] === undefined ? id : titles[id];
    }

    function show(v, kinds, total, modes) {
      f = v;
      kindsShown = kinds || {};
      if (modes) modesShown = modes;
      box.textContent = "";
      made = 0;
      box.hidden = !LQ().used(v);
      if (box.hidden) return;
      if (total !== undefined) {
        box.dataset.total = String(total);
      }
      var count = box.dataset.total;
      // With nothing left, the table's own message says so and has the
      // one "Clear filters": the chips stay, to take one filter off.
      var none = count === "0";
      if (count !== undefined && !none) {
        box.appendChild(KW.el("span", "fpill-n", KW.num(count) +
          (count === "1" ? " issue matches" : " issues match")));
      }
      if (v.like) {
        box.appendChild(chip("Similar to", likeTitle(v.like), function () { return { like: "" }; }));
      }
      LQ().MULTI.forEach(function (key) {
        if (!NAMES[key]) return;
        v[key].forEach(function (x) {
          box.appendChild(chip(NAMES[key], word(key, x, kinds || {}, modesShown),
            without(key, x), key === "namespace" || key === "node"));
        });
      });
      if (none) return;
      var clear = KW.el("button", "link fpill-clear", "Clear all");
      clear.type = "button";
      clear.dataset.fclear = "";
      clear.dataset.fk = "chip:" + made;
      clear.addEventListener("click", function () {
        var keep = LQ().blank();
        ["status", "sort", "dir", "pageSize"].forEach(function (k) { keep[k] = f[k]; });
        onChange(keep);
      });
      box.appendChild(clear);
    }

    return { show: show };
  };
})();
