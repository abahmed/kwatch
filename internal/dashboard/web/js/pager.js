/* kwatch dashboard: the pager under the Issues table. "Showing 51–100
   of 4,812", First, Previous, the page numbers (with … for the pages
   left out), Next, Last, and the rows-per-page picker. issues.js owns
   it: it paints a list response and calls onPage(n) or onSize(n). It
   is hidden while everything fits on one page. */
(function () {
  "use strict";
  var KW = window.KW;

  var n = function (x) { return KW.num(x); };

  /* numbers: the page numbers shown around page of total, with 0 where
     pages are left out: 1 … 4 5 6 … 20. */
  function numbers(page, total) {
    var out = [];
    for (var i = 1; i <= total; i++) {
      if (i === 1 || i === total || Math.abs(i - page) <= 1) out.push(i);
      else if (out[out.length - 1] !== 0) out.push(0);
    }
    return out;
  }

  KW.pager = function (box, hooks) {
    var range = KW.$("[data-range]", box), btns = KW.$("[data-pages]", box);
    var size = KW.$("[data-size]", box);

    function button(text, to, label, current) {
      var b = KW.el("button", "btn btn-sm", text);
      b.type = "button";
      b.dataset.fk = "page:" + text;
      if (label) b.setAttribute("aria-label", label);
      if (current) b.setAttribute("aria-current", "page");
      b.disabled = !to;
      b.addEventListener("click", function () { if (!current) hooks.onPage(to); });
      return b;
    }

    /* paint shows a list response for page, pageSize. */
    function paint(data, page, pageSize) {
      var total = data.total || 0, last = data.totalPages || 0;
      var from = (page - 1) * pageSize, shown = (data.incidents || []).length;
      range.textContent = shown ? "Showing " + n(from + 1) + "–" +
        n(from + shown) + " of " + n(total) : "Showing 0 of " + n(total);
      btns.textContent = "";
      btns.appendChild(button("First", page > 1 ? 1 : 0));
      btns.appendChild(button("← Previous", page > 1 ? Math.min(page - 1, last || 1) : 0));
      numbers(Math.min(page, last), last).forEach(function (p) {
        if (!p) return btns.appendChild(KW.el("span", "pager-gap", "…"));
        btns.appendChild(button(n(p), p, "Page " + p, p === page));
      });
      btns.appendChild(button("Next →", page < last ? page + 1 : 0));
      btns.appendChild(button("Last", page < last ? last : 0));
      size.value = String(pageSize);
      box.hidden = last <= 1 && page <= 1;
    }

    size.addEventListener("change", function () {
      hooks.onSize(parseInt(size.value, 10) || 50);
    });

    return { paint: paint };
  };
})();
