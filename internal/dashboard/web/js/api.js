/* kwatch dashboard: talking to the server (KW.api), copying text, and
   the shared error and starting panels. core.js loads first. */
(function () {
  "use strict";
  var KW = window.KW;

  /* ---- API ---- */
  /* api returns {status, data, retryAfter, etag}; status 0 is a network
     failure (or opts.signal aborted it). opts.etag is sent as
     If-None-Match: an unchanged list answers 304 with no data. The Date
     header corrects the browser clock. */
  KW.api = function (path, opts) {
    opts = opts || {};
    var init = { method: opts.method || "GET", credentials: "same-origin",
      cache: "no-store", headers: { Accept: "application/json" } };
    if (opts.body !== undefined) {
      init.headers["Content-Type"] = "application/json";
      init.body = JSON.stringify(opts.body);
    }
    if (opts.etag) init.headers["If-None-Match"] = opts.etag;
    var ctl = window.AbortController ? new AbortController() : null;
    var timer = 0, onAbort = null;
    // settle frees the timer and the caller's listener once we are done.
    function settle() {
      window.clearTimeout(timer);
      if (onAbort) opts.signal.removeEventListener("abort", onAbort);
    }
    if (ctl) {
      init.signal = ctl.signal;
      timer = window.setTimeout(function () { ctl.abort(); }, 15000);
      if (opts.signal) {
        onAbort = function () { ctl.abort(); };
        opts.signal.addEventListener("abort", onAbort, { once: true });
      }
    }
    // Any failure, reading the body included, answers status 0.
    return fetch(path, init).then(function (res) {
      var date = Date.parse(res.headers.get("Date") || "");
      if (!isNaN(date)) KW.skew = date - Date.now();
      var retry = parseInt(res.headers.get("Retry-After") || "", 10);
      var etag = res.headers.get("ETag") || "";
      return res.text().then(function (text) {
        var data = null;
        try { data = text ? JSON.parse(text) : null; } catch (e) { data = null; }
        settle();
        if (data && typeof data === "object") fixTitles(data, 0);
        // The page that shows kwatch's own error panels needs its name.
        if (res.status === 200 && data && data.kwatchNamespace &&
            /^\/api\/status(\?|$)/.test(path)) {
          KW.self = { namespace: data.kwatchNamespace,
            deployment: data.kwatchDeployment || "kwatch" };
        }
        return { status: res.status, data: data, etag: etag,
          retryAfter: isNaN(retry) ? 0 : retry };
      });
    }).then(null, function () {
      settle();
      return { status: 0, data: null, retryAfter: 0, etag: "" };
    });
  };

  /* fixTitles puts every title of an answer through KW.problemTitle,
     a few levels deep (lists of issues and their links). */
  function fixTitles(o, depth) {
    if (depth > 4 || !o || typeof o !== "object") return;
    if (Array.isArray(o)) {
      o.forEach(function (x) { fixTitles(x, depth + 1); });
      return;
    }
    if (typeof o.title === "string") o.title = KW.problemTitle(o.title, o);
    Object.keys(o).forEach(function (k) {
      if (o[k] && typeof o[k] === "object") fixTitles(o[k], depth + 1);
    });
  }

  /* ---- copy ---- */
  KW.copy = function (button, text, label) {
    function done() {
      KW.announce("Copied");
      if (button.classList.contains("copy-ico")) {
        // An icon button shows a check for a moment; its name stays.
        button.classList.add("is-copied");
        window.clearTimeout(button._copied);
        button._copied = window.setTimeout(function () {
          button.classList.remove("is-copied");
        }, 1600);
        return;
      }
      button.textContent = "Copied";
      window.setTimeout(function () { button.textContent = label || "Copy"; }, 2000);
    }
    function fallback() {
      var target = button.parentNode.querySelector("code, .logs");
      if (!target || !target.isConnected) return;
      var range = document.createRange();
      range.selectNodeContents(target);
      var sel = window.getSelection();
      sel.removeAllRanges();
      sel.addRange(range);
    }
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(done, fallback);
    } else {
      fallback();
    }
  };

  /* KW.copyButton is the one copy button of every command and code
     box: a small copy icon at the box's top right (the box keeps room
     for it), named "Copy <what>" for screen readers and in its
     tooltip. After a copy it shows a check for a moment and "Copied" is
     announced (KW.copy). text is the text, or a function that reads it
     at the click. */
  KW.copyButton = function (text, what) {
    var b = KW.el("button", "copy-ico");
    b.type = "button";
    b.setAttribute("aria-label", "Copy " + (what || "command"));
    b.title = "Copy " + (what || "command");
    b.appendChild(KW.icon("copy")).classList.add("copy-i");
    b.appendChild(KW.icon("check")).classList.add("copy-ok");
    b.addEventListener("click", function () {
      KW.copy(b, typeof text === "function" ? text() : text);
    });
    return b;
  };

  /* cmd is a command box: one line, scrolls inside, the copy icon
     copies it. */
  KW.cmd = function (command) {
    var box = KW.el("div", "cmd");
    var code = KW.el("code", "", command);
    code.tabIndex = 0;
    box.appendChild(code);
    box.appendChild(KW.copyButton(command));
    return box;
  };

  KW.wireCopy = function (root) {
    KW.$$("[data-copy]", root).forEach(function (b) {
      b.addEventListener("click", function () {
        KW.copy(b, b.parentNode.querySelector("code").textContent);
      });
    });
  };

  /* ---- shared states ---- */
  KW.self = { namespace: "kwatch", deployment: "kwatch" };

  /* panel renders one of the shared error or starting panels.
     kind: starting | network | limit | error. */
  KW.panel = function (kind, status, onRetry) {
    var frag = KW.clone("tpl-panel");
    var p = frag.firstElementChild;
    // Starting is news, not a failure: read politely.
    if (kind === "starting") p.setAttribute("role", "status");
    var retry = KW.$("[data-retry]", p);
    var texts = {
      starting: ["kwatch is loading what it knows",
        "This page updates by itself.", "Try now"],
      network: ["Can't reach kwatch",
        "Check your connection. This page tries again by itself.", "Try now"],
      limit: ["Too many requests",
        "Wait a minute. This page tries again by itself.", ""],
      error: ["The dashboard hit an error",
        "Your cluster is not affected. kwatch's own logs say why:",
        "Try again"]
    }[kind];
    KW.$("[data-heading]", p).textContent = texts[0];
    KW.$("[data-body]", p).textContent = texts[1];
    if (kind === "error") {
      var cmd = KW.$("[data-cmd]", p);
      cmd.hidden = false;
      KW.$("code", cmd).textContent = "kubectl -n " + KW.self.namespace +
        " logs deploy/" + KW.self.deployment + " --tail=50";
      KW.wireCopy(p);
      var code = KW.$("[data-code]", p);
      code.hidden = false;
      code.textContent = "Error " + status;
    }
    if (texts[2] && onRetry) {
      retry.hidden = false;
      retry.textContent = texts[2];
      retry.addEventListener("click", onRetry);
    }
    return p;
  };

  /* kindOf sorts a failed API status into a shared state. */
  KW.kindOf = function (status) {
    if (status === 0) return "network";
    if (status === 503) return "starting";
    if (status === 429) return "limit";
    return "error";
  };

  /* retryDelay: 5 s for the first tries while starting, then 15 s;
     60 s after 429. */
  KW.retryDelay = function (kind, attempt) {
    if (kind === "limit") return 60000;
    if (kind === "starting") return attempt < 6 ? 5000 : 15000;
    return 15000;
  };
})();
