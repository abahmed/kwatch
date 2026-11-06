/* kwatch dashboard: the Issues view options (text size and high
   contrast included) and saved views of this browser, kept with KW.store (localStorage key kwatch.view). Not
   cookies: the dashboard keeps no per-person state on the server and
   these settings never need to reach it. Without storage they last
   until the page is reloaded. No DOM here. */
(function () {
  "use strict";
  var KW = window.KW;
  var LQ = function () { return KW.listQuery; };

  /* The table's columns. Severity and title are always shown; the
     title also carries how often an issue failed again (× 3) and the
     New / Changed mark. "When" is how long an active issue has been
     going on, or when a resolved one resolved (issuetable.js). */
  var COLUMNS = [
    { key: "title", label: "Issue", sort: "title", always: true },
    { key: "sev", label: "Severity", sort: "severity", always: true },
    { key: "where", label: "Where", sort: "namespace" },
    { key: "status", label: "Status", sort: "status" },
    { key: "last", label: "When (happening now or resolved)", sort: "last" },
    { key: "ack", label: "Acknowledged", short: "Acked", sort: "acked" },
    { key: "started", label: "Start time", sort: "started" },
    { key: "dur", label: "Duration", sort: "duration" }
  ];
  var SHOWN = ["title", "sev", "where", "status", "last"];
  /* Column presets of the Columns panel. */
  var PRESETS = [
    { id: "compact", label: "Compact", cols: ["title", "sev", "where", "last"] },
    { id: "standard", label: "Standard", cols: SHOWN },
    { id: "all", label: "Everything", cols: COLUMNS.map(function (c) { return c.key; }) }
  ];
  /* Widths a column may be dragged to, in px. */
  var MIN_W = 56, MAX_W = 640;
  var MAX_VIEWS = 20;

  function known(key) {
    return COLUMNS.some(function (c) { return c.key === key; });
  }

  function column(key) {
    return COLUMNS.filter(function (c) { return c.key === key; })[0];
  }

  /* A column kept from before Namespace and Object became one. */
  var MERGED = { ns: "where", obj: "where" };
  function migrate(keys) {
    return keys.map(function (k) { return MERGED[k] || k; })
      .filter(function (k, i, all) { return known(k) && all.indexOf(k) === i; });
  }

  /* order: every column key in the order chosen (new columns at the
     end). The issue is always first: its row carries the severity strip. */
  function order() {
    var saved = KW.store.get("order");
    var keys = Array.isArray(saved) ? migrate(saved) : [];
    COLUMNS.forEach(function (c) { if (keys.indexOf(c.key) < 0) keys.push(c.key); });
    keys = keys.filter(function (k, i) { return keys.indexOf(k) === i && k !== "title"; });
    return ["title"].concat(keys);
  }

  /* columns: the visible column keys, in table order. */
  function columns() {
    var saved = KW.store.get("cols");
    var keys = Array.isArray(saved) ? migrate(saved) : SHOWN;
    return order().filter(function (k) {
      return column(k).always || keys.indexOf(k) >= 0;
    });
  }

  /* setColumns shows the keys (in the order kept) and, given one, puts
     the columns in that order. */
  function setColumns(keys, newOrder) {
    if (newOrder) KW.store.set("order", migrate(newOrder));
    KW.store.set("cols", order().filter(function (k) { return keys.indexOf(k) >= 0; }));
  }

  /* widths: the widths dragged, by column key. */
  function widths() {
    var w = KW.store.get("widths"), out = {};
    if (!w || typeof w !== "object") return out;
    Object.keys(w).forEach(function (k) {
      var px = Number(w[k]);
      if (known(k) && px >= MIN_W && px <= MAX_W) out[k] = Math.round(px);
    });
    return out;
  }

  function setWidth(key, px) {
    var w = widths();
    if (px === undefined) delete w[key];
    else w[key] = Math.max(MIN_W, Math.min(MAX_W, Math.round(px)));
    KW.store.set("widths", Object.keys(w).length ? w : undefined);
  }

  function resetColumns() {
    ["cols", "order", "widths"].forEach(function (k) { KW.store.set(k, undefined); });
  }

  /* useView puts a saved view's columns back. */
  function useView(v) {
    if (Array.isArray(v.order)) KW.store.set("order", migrate(v.order));
    if (Array.isArray(v.cols)) KW.store.set("cols", migrate(v.cols));
    KW.store.set("widths", v.widths && typeof v.widths === "object" ? v.widths : undefined);
  }

  function oneOf(v, allowed, fallback) {
    return allowed.indexOf(v) >= 0 ? v : fallback;
  }

  /* options: every view option, with the defaults filled in. */
  function options() {
    var s = KW.store;
    // A stored sort the page no longer knows would make every list
    // read fail with 400: unknown values are dropped.
    var sort = String(s.get("sort") || "").split(":");
    var key = oneOf(sort[0], LQ().SORTS, "");
    return { cols: columns(),
      density: oneOf(s.get("density"), ["comfortable", "compact"], "comfortable"),
      size: oneOf(s.get("size"), LQ().SIZES, 50),
      status: oneOf(s.get("status"), LQ().STATUSES, "active"),
      sort: key, dir: key ? oneOf(sort[1], ["asc", "desc"], "") : "",
      text: oneOf(s.get("text"), KW.TEXT_SIZES, "m"),
      contrast: s.get("contrast") === "more" };
  }

  /* setLook keeps the text size (s, m, l, xl) and high contrast, and
     applies them to the page at once (look.js). */
  function setLook(text, contrast) {
    KW.store.set("text", text === "m" ? undefined : text);
    KW.store.set("contrast", contrast ? "more" : undefined);
    KW.look(text, contrast ? "more" : "");
  }

  /* start: the view "/issues" opens with when the address has none. */
  function start() {
    var v = defaultView();
    if (v) return LQ().parse(v.query);
    var o = options(), f = LQ().blank();
    f.status = o.status;
    f.sort = o.sort;
    f.dir = o.sort ? o.dir : "";
    f.pageSize = o.size;
    return f;
  }

  /* reset puts every view option back, Local times included (the
     sidebar switch redraws the page in them). */
  function reset() {
    ["cols", "order", "widths", "density", "size", "status",
      "sort", "text", "contrast"].forEach(function (k) {
      KW.store.set(k, undefined);
    });
    KW.look("m", "");
    try { window.localStorage.removeItem("kwatch.tz"); } catch (e) { /* private mode */ }
    if (KW.utc) {
      var local = document.querySelector('.tz [data-tz="local"]');
      if (local) local.click();
    }
    views().forEach(function (v) { v.isDefault = false; });
    KW.store.set("views", views());
  }

  /* ---- saved views: {name, query, cols, order, widths, isDefault} ---- */
  function views() {
    var list = KW.store.get("views");
    if (!Array.isArray(list)) return [];
    return list.filter(function (v) {
      return v && typeof v.name === "string" && v.name &&
        typeof v.query === "string" && v.query.charAt(0) === "?";
    }).slice(0, MAX_VIEWS);
  }

  function find(list, name) {
    for (var i = 0; i < list.length; i++) if (list[i].name === name) return i;
    return -1;
  }

  /* saveView stores (or replaces) a view under name, with the
     columns shown now. It returns true, false (no name, or 20 views)
     or "full" when this browser could not keep it. */
  function saveView(name, query) {
    name = String(name || "").trim().slice(0, 40);
    if (!name) return false;
    var list = views(), at = find(list, name);
    var v = { name: name, query: query, cols: columns(), order: order(),
      widths: widths(), isDefault: at >= 0 && list[at].isDefault };
    if (at >= 0) list[at] = v;
    else if (list.length < MAX_VIEWS) list.push(v);
    else return false;
    var before = views();
    if (!KW.store.set("views", list)) {
      // Not kept: leave it out of the list too, or it would vanish on reload.
      KW.store.set("views", before);
      return "full";
    }
    return true;
  }

  function removeView(name) {
    var list = views(), at = find(list, name);
    if (at >= 0) list.splice(at, 1);
    KW.store.set("views", list);
  }

  function renameView(name, to) {
    to = String(to || "").trim().slice(0, 40);
    var list = views(), at = find(list, name);
    if (at < 0 || !to || (to !== name && find(list, to) >= 0)) return false;
    list[at].name = to;
    KW.store.set("views", list);
    return true;
  }

  /* setDefault marks the view "/issues" opens; "" unmarks them all. */
  function setDefault(name) {
    var list = views();
    list.forEach(function (v) { v.isDefault = v.name === name; });
    KW.store.set("views", list);
  }

  function defaultView() {
    var list = views();
    for (var i = 0; i < list.length; i++) if (list[i].isDefault) return list[i];
    return null;
  }

  KW.prefs = { COLUMNS: COLUMNS, PRESETS: PRESETS, MIN_W: MIN_W,
    MAX_W: MAX_W, column: column, columns: columns, order: order,
    setColumns: setColumns, widths: widths, setWidth: setWidth,
    resetColumns: resetColumns, useView: useView, options: options,
    setLook: setLook,
    start: start, reset: reset, views: views, saveView: saveView,
    removeView: removeView, renameView: renameView, setDefault: setDefault,
    defaultView: defaultView,
    set: function (k, v) { KW.store.set(k, v); } };
})();
