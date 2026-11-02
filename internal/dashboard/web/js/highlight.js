/* kwatch dashboard: search matches marked in the text shown, and
   whether the reader is selecting text. The marks are built with DOM
   calls (text nodes and <mark>), never as HTML, and found with
   indexOf, not a pattern, so any character is safe. */
(function () {
  "use strict";
  var KW = window.KW;

  /* terms: the plain words and quoted phrases of a search (the
     shortcuts and -excluded words are not in the text), lower case. */
  KW.searchTerms = function (q) {
    return KW.searchQuery.parse(q).terms.map(function (t) {
      return t.toLowerCase();
    }).filter(function (t) { return t.length > 0; });
  };

  /* ranges: where terms are in text, as sorted [start, end) pairs with
     overlaps merged. Case-insensitive; text whose lower case has
     another length (a few scripts) gets none. */
  function ranges(text, terms) {
    var low = text.toLowerCase(), out = [];
    if (low.length !== text.length) return out;
    terms.forEach(function (t) {
      for (var at = low.indexOf(t); at >= 0; at = low.indexOf(t, at + t.length)) {
        out.push([at, at + t.length]);
      }
    });
    out.sort(function (a, b) { return a[0] - b[0] || a[1] - b[1]; });
    var merged = [];
    out.forEach(function (r) {
      var last = merged[merged.length - 1];
      if (last && r[0] <= last[1]) last[1] = Math.max(last[1], r[1]);
      else merged.push(r.slice());
    });
    return merged;
  }

  /* highlight sets node's text to text, with each match of terms in a
     <mark>. */
  KW.highlight = function (node, text, terms) {
    node.textContent = "";
    var at = 0;
    ranges(text || "", terms || []).forEach(function (r) {
      if (r[0] > at) node.appendChild(document.createTextNode(text.slice(at, r[0])));
      node.appendChild(KW.el("mark", "hl", text.slice(r[0], r[1])));
      at = r[1];
    });
    if (at < (text || "").length) node.appendChild(document.createTextNode(text.slice(at)));
  };

  /* selecting: some text inside root is selected. */
  KW.selecting = function (root) {
    var sel = window.getSelection && window.getSelection();
    if (!root || !sel || sel.isCollapsed || !sel.rangeCount) return false;
    return root.contains(sel.anchorNode) || root.contains(sel.focusNode);
  };
})();
