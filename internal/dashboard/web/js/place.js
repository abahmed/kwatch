/* kwatch dashboard: back to the exact place in the Issues table. When
   the table is left (for an issue page, or anywhere), its address,
   scroll position and the row last focused or opened are kept in
   sessionStorage (key kwatch.place, this tab only). Coming back to the
   same address puts the page there again, after a reload too. Without
   storage it works until the page is reloaded. */
(function () {
  "use strict";
  var KW = window.KW;
  var KEY = "kwatch.place";
  var memory = null;

  function valid(v) {
    return !!v && typeof v.url === "string" && typeof v.y === "number" &&
      typeof v.id === "string";
  }

  KW.place = {
    /* save keeps where the table at url was left. */
    save: function (url, y, id) {
      memory = { url: url, y: Math.max(0, Math.round(y)), id: id || "" };
      try { sessionStorage.setItem(KEY, JSON.stringify(memory)); } catch (e) { /* this page only */ }
    },
    /* take returns, once, the place kept for url, or null. */
    take: function (url) {
      var v = memory;
      try { v = JSON.parse(sessionStorage.getItem(KEY) || "null") || v; } catch (e) { v = memory; }
      if (!valid(v) || v.url !== url) return null;
      memory = null;
      try { sessionStorage.removeItem(KEY); } catch (e) { /* this page only */ }
      return v;
    }
  };
})();
