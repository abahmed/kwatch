/* kwatch dashboard: the New / Changed mark, the sentence that says what
   kwatch watches, what the Issues table shows when there is nothing to
   show (the empty states), and keeping the keyboard focus in place when
   part of a page is drawn again. */
(function () {
  "use strict";
  var KW = window.KW;

  /* visitChip: "New" or "Changed" since the last visit (visits.js),
     with what changed in its tooltip. */
  function visitChip(mark) {
    var d = KW.visits.describe(mark);
    var c = KW.chip(mark.what === "new" ? "c-seen-new" : "c-seen-changed",
      d.label, d.title);
    c.setAttribute("aria-label", d.title);
    return c;
  }

  /* scopeLine: what kwatch watches, from /api/status, in the same words
     on the Overview and the Issues page: "Watching every namespace
     except kube-system". count, when known, is how many namespaces
     that is now. */
  KW.scopeLine = function (status, count) {
    var w = (status && status.watchedNamespaces) || ["*"];
    var ex = (status && status.excludedNamespaces) || [];
    function names(list) {
      if (list.length <= 3) return list.join(", ");
      return list.slice(0, 3).join(", ") + " and " + KW.num(list.length - 3) + " more";
    }
    var now = typeof count === "number" ?
      " (" + KW.plural(count, "namespace", "namespaces") + " now)" : "";
    if (w.length === 1 && w[0] === "*") {
      return "Watching every namespace" + (ex.length ? " except " + names(ex) : "") + now;
    }
    return "Watching " + names(w) + now;
  };

  /* ---- empty states ---- */
  function clearBlock(lines, compact) {
    var box = KW.el("div", "clear" + (compact ? " compact" : ""));
    lines.forEach(function (l) { if (l) box.appendChild(l); });
    return box;
  }

  function late(status) {
    var n = KW.el("div", "notice n-degraded");
    n.setAttribute("role", "status");
    n.appendChild(KW.el("b", "", "kwatch hasn't checked in a while."));
    var p = KW.el("p", "", "Last check: " + KW.rel(status.lastScan) + ". ");
    var a = KW.el("a", "ulink", "See kwatch status.");
    a.href = "/kwatch";
    a.dataset.link = "";
    p.appendChild(a);
    n.appendChild(p);
    return n;
  }

  /* activeEmpty: nothing active. resolved is how many issues resolved
     in the last 7 days; the link to them shows only when there are. */
  function activeEmpty(status, resolved) {
    if (!status) {
      return clearBlock([KW.el("p", "l1", "Nothing is listed as failing.")], true);
    }
    if (!KW.valid(status.lastScan)) {
      return clearBlock([KW.el("p", "l1", "kwatch is doing its first check."),
        KW.el("p", "l2", "Problems show up here when it finds any.")], true);
    }
    if (status.lastScanLate) return late(status);
    var l1 = KW.el("p", "l1", "Nothing is failing. Last check: ");
    l1.appendChild(KW.timeEl(status.lastScan, "", true));
    l1.appendChild(document.createTextNode("."));
    var l3 = null;
    if (resolved > 0) {
      l3 = KW.el("p", "l3");
      var link = KW.el("a", "ulink", "See the " + KW.plural(resolved,
        "issue", "issues") + " resolved this week");
      link.href = "/issues?status=resolved";
      link.dataset.link = "";
      l3.appendChild(link);
    }
    return clearBlock([KW.el("h2", "", "All clear"), l1,
      KW.el("p", "l2", KW.scopeLine(status) + "."), l3]);
  }

  function resolvedEmpty() {
    return clearBlock([KW.el("p", "l1", "Nothing was fixed in the last 7 days."),
      KW.el("p", "l2", "Issues that stop failing show up here for 7 days.")], true);
  }

  /* ---- keeping the focus ---- */
  /* keepFocus remembers the data-fk key of the element focused inside
     root. When part of root is drawn again and the focus was lost with
     the old element, restore() focuses the new element with the same
     key; for a key that ends in ":<n>" (the n-th filter chip) the one
     before it, else fallback(key). Clicking elsewhere forgets the key. */
  KW.keepFocus = function (root) {
    var key = "";
    function onIn(e) {
      var el = e.target.closest && e.target.closest("[data-fk]");
      key = el && root.contains(el) ? el.dataset.fk : "";
    }
    function onOut(e) {
      var el = e.target;
      window.setTimeout(function () {
        if (el.isConnected && !root.contains(document.activeElement)) key = "";
      }, 0);
    }
    root.addEventListener("focusin", onIn);
    root.addEventListener("focusout", onOut);
    function find(k) {
      var el = root.querySelector('[data-fk="' + CSS.escape(k) + '"]');
      return el && !el.disabled ? el : null;
    }
    return {
      restore: function (fallback) {
        var a = document.activeElement;
        if (!key || (a && a !== document.body && a !== root && a.isConnected)) return;
        var el = find(key), m = /^(.*):(\d+)$/.exec(key);
        for (var i = m ? +m[2] - 1 : -1; !el && i >= 0; i--) el = find(m[1] + ":" + i);
        if (!el && fallback) el = fallback(key);
        if (el) el.focus({ preventScroll: true });
      },
      stop: function () {
        root.removeEventListener("focusin", onIn);
        root.removeEventListener("focusout", onOut);
      }
    };
  };

  KW.visitChip = visitChip;
  KW.activeEmpty = activeEmpty;
  KW.resolvedEmpty = resolvedEmpty;
})();
