/* kwatch dashboard: grouped objects and paging notes. On the issue
   page: the objects a removed or stopped controller left behind (one
   issue for all of them, in the Impact tab, with a line in "What's
   wrong?" that opens it) and one plain line beside the alert level
   when kwatch lowered or capped it or holds its page. On the Overview:
   the old problems kwatch opened no issue for, folded behind one quiet
   line, and "Paging held while the cluster scales". On the kwatch
   health page: "Non-production cluster: nothing pages". */
(function () {
  "use strict";
  var KW = window.KW;
  var P = KW.TERMS.paging;
  var FIRST = 10;

  /* place: "Secret · shop/web-tls", the kind in words. */
  function place(o) {
    return (o.kind ? o.kind + " · " : "") + (o.namespace ? o.namespace + "/" : "") + o.name;
  }

  /* objectRow: kind · namespace/name, then what is wrong with it. */
  function objectRow(o, tail) {
    var li = KW.el("li", "lo-row");
    li.appendChild(KW.wrapName(place(o), "lo-name mono"));
    var what = li.appendChild(KW.el("span", "lo-what", o.what || o.reason || ""));
    if (o.reason && o.what) what.title = o.reason;
    if (tail) li.appendChild(tail);
    return li;
  }

  /* wasAll: the reader chose "Show all" in the list that box holds.
     Asked before a redraw empties box, so the choice outlives it. */
  function wasAll(box) { return !!KW.$(".lo-list[data-all]", box); }

  /* KW.objectList: the first ten rows of list and "Show all N" for the
     rest (every row when all is set: the reader already asked for
     them); total is how many there are when the list was cut, and more
     the words for the cut, ["more object", "more objects"]. */
  KW.objectList = function (box, list, total, row, all, more) {
    var ul = box.appendChild(KW.el("ul", "lo-list"));
    if (all) ul.dataset.all = "";
    list.forEach(function (o, i) {
      var li = ul.appendChild(row(o));
      li.hidden = !all && i >= FIRST;
    });
    if (!all && list.length > FIRST) {
      var b = box.appendChild(KW.el("button", "btn btn-sm lo-all", "Show all " + KW.num(list.length)));
      b.type = "button";
      b.addEventListener("click", function () {
        KW.$$("li", ul).forEach(function (li) { li.hidden = false; });
        ul.dataset.all = "";
        b.remove();
      });
    }
    var cut = (total || 0) - list.length;
    more = more || ["more object", "more objects"];
    if (cut > 0) box.appendChild(KW.el("p", "ev-cap", "+" + KW.plural(cut, more[0], more[1]) + " not shown."));
  };

  /* KW.leftovers fills the Impact tab's "Objects left behind". */
  KW.leftovers = function (root, inc) {
    var sec = KW.$('[data-fold="leftovers"]', root);
    if (!sec) return;
    var body = KW.$(".fold-body", sec), list = inc.leftovers || [];
    var all = wasAll(body);
    body.textContent = "";
    sec.hidden = !list.length;
    if (sec.hidden) return;
    KW.$(".sec-h", sec).textContent = P.leftHead + " (" + KW.num(inc.leftoverObjects || list.length) + ")";
    body.appendChild(KW.el("p", "ev-cap", P.leftTitle));
    KW.objectList(body, list, inc.leftoverObjects, function (o) { return objectRow(o); }, all);
  };

  /* KW.leftoverLine: in "What's wrong?", "12 objects left behind · See
     which →", which opens the list. */
  KW.leftoverLine = function (dd, root, inc) {
    var n = inc.leftoverObjects || (inc.leftovers || []).length;
    if (!n) return;
    var p = dd.appendChild(KW.el("p", "lo-line", KW.plural(n, "object", "objects") + " left behind · "));
    var b = p.appendChild(KW.el("button", "link", "See which →"));
    b.type = "button";
    b.addEventListener("click", function () { KW.openFold(root, "leftovers"); });
  };

  /* KW.levelNote: why the alert level is not what the failure alone
     would give: lowered, capped or not paged, or a page held while the
     cluster scales. Before those, that kwatch has held a finding for a
     while over a kind it cannot read now (why.unverifiedReason). The
     first sentence only; the whole in the tooltip (the Alerts tab says
     it in full). "" when there is nothing. */
  KW.levelNote = function (inc) {
    var why = inc.why || {};
    if (why.unverifiedReason && inc.state !== "resolved") return why.unverifiedReason;
    if (why.pagingHeldReason && inc.state !== "resolved") return why.pagingHeldReason;
    var r = why.alertLevelReason || "";
    if (!/^(Lowered to|Not paged)|^[^:.]* instead of /.test(r)) return "";
    return r;
  };
  /* KW.heldUntil: when the held page of inc goes out if no node
     changes again (the API's why.pagingHeldUntil); "" without one. */
  KW.heldUntil = function (inc) {
    var why = inc.why || {};
    if (!why.pagingHeldReason || inc.state === "resolved") return "";
    return KW.valid(why.pagingHeldUntil) ? why.pagingHeldUntil : "";
  };
  /* KW.heldWords: "the page is held while the cluster scales, until
     ~14:20", for "Who was alerted?"; "" when no page is held. */
  KW.heldWords = function (inc) {
    var why = inc.why || {};
    if (!why.pagingHeldReason || inc.state === "resolved") return "";
    var until = KW.heldUntil(inc);
    return P.heldTold + (until ? ", until ~" + KW.when(until) : "");
  };
  /* KW.levelLine puts that note on its own line under the level chip
     (head is the row of the chip and the copy buttons). */
  KW.levelLine = function (head, inc) {
    var old = head.parentNode.querySelector(":scope > [data-level-note]");
    if (old) old.remove();
    var text = KW.levelNote(inc);
    if (!text) return;
    var first = text.replace(/^(.*?[.])\s.*$/, "$1").replace(/\.$/, "");
    var p = KW.el("p", "ds-lvl", first);
    p.dataset.levelNote = "";
    p.title = text;
    var until = KW.heldUntil(inc);
    if (until) {
      p.appendChild(document.createTextNode(" · until ~"));
      p.appendChild(KW.hm(until));
    }
    head.parentNode.insertBefore(p, head.nextSibling);
  };

  /* age: "45 days", "about 5 months", "about 2 years". */
  function about(n, word) { return n === 1 ? "about a " + word : "about " + KW.num(n) + " " + word + "s"; }
  /* The same words as the issue's "Lowered to Low" line (spokenDays in
     internal/incident/old_problem.go): a year starts at 365.25 days. */
  function age(days) {
    if (days < 60) return KW.plural(days, "day", "days");
    if (days < 365.25) return about(Math.round(days / 30.44), "month");
    return about(Math.round(days / 365.25), "year");
  }

  function oldRow(o) {
    var t = KW.el("span", "lo-age", "for " + age(o.ageDays || 0));
    if (KW.valid(o.since)) t.title = "Failing since " + KW.full(o.since);
    return objectRow(o, t);
  }

  /* KW.oldProblems: one quiet line, "3 old problems not shown (…)",
     that unfolds into the list with how long each has failed. Hidden
     when there are none. It keeps the fold open, and "Show all", across
     redraws. op.days is the configured age of an old problem; without
     it the line names no number. */
  KW.oldProblems = function (box, op) {
    var was = KW.$("details", box), open = !!(was && was.open), all = wasAll(box);
    box.textContent = "";
    box.hidden = !op || !op.count;
    if (box.hidden) return;
    var det = box.appendChild(KW.el("details", "ov-old"));
    det.open = open;
    var sum = det.appendChild(KW.el("summary"));
    sum.appendChild(document.createTextNode(KW.plural(op.count, "old problem", "old problems") +
      " not shown (" + (op.days > 0 ? P.oldDays.replace("{days}", KW.num(op.days)) : P.old) + ") "));
    var show = sum.appendChild(KW.el("span", "ulink ov-old-show", open ? "Hide them" : "Show them"));
    det.addEventListener("toggle", function () { show.textContent = det.open ? "Hide them" : "Show them"; });
    KW.objectList(det, op.items || [], op.count, oldRow, all, ["more problem", "more problems"]);
  };

  /* KW.holdLine: "Paging held while the cluster scales · until ~14:20". */
  KW.holdLine = function (hold) {
    if (!hold) return null;
    var p = KW.el("p", "ov-hold", P.held);
    if (KW.valid(hold.expectedEnd)) {
      var at = p.appendChild(KW.el("span", "ov-hold-at", " · until ~"));
      at.appendChild(KW.hm(hold.expectedEnd));
    }
    p.title = (hold.reason || P.held) + (KW.valid(hold.since) ? ", since " + KW.full(hold.since) : "") +
      ". Urgent alerts wait until the nodes stop coming and going.";
    return p;
  };

  /* KW.nonProdLine: the badge on the kwatch health page. */
  KW.nonProdLine = function (k) {
    if (!k || !k.nonProduction) return null;
    var p = KW.el("p", "kp-np");
    p.appendChild(KW.chip("c-acked", P.nonProd,
      "app.environment marks this cluster as non-production, so kwatch alerts your channels but never pages anyone."));
    return p;
  };
})();
