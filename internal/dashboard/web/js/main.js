/* kwatch dashboard: router, shell, sign in and the live refresh loop. */
(function () {
  "use strict";
  var KW = window.KW;
  var view = null;
  var shellEl = null;
  var current = "";
  /* The Issues table's address last shown, for "Back" on an issue. */
  KW.lastList = "/issues";

  /* ---- routing ---- */
  /* The list filters of the old Active issues page at "/". */
  var LIST_KEYS = ["q", "severity", "namespace", "state", "age", "since",
    "sort", "dir", "group", "kind", "acked", "status", "offset", "limit",
    "page", "pageSize", "release"];

  /* moved is the new address of an old one (the server redirects them
     too; this covers Back to an old history entry), or "". */
  function moved(path, search) {
    var p = new URLSearchParams(search);
    if (path === "/status" || path === "/insights") return "/";
    if (path === "/releases") return "/issues";
    if (path === "/resolved") {
      p.delete("status");
      var rest = p.toString();
      return "/issues?status=resolved" + (rest ? "&" + rest : "");
    }
    if (path === "/" && LIST_KEYS.some(function (k) { return p.has(k); })) {
      return "/issues" + search;
    }
    return "";
  }

  function route(path) {
    if (path === "/signin") return { name: "signin" };
    if (path === "/") return { name: "status" };
    if (path === "/issues") return { name: "issues" };
    if (path === "/kwatch") return { name: "kwatch" };
    if (path === "/activity") return { name: "activity" };
    var m = /^\/issues\/([^/]+)$/.exec(path);
    if (m) {
      // A malformed escape (%E0) is an ID no issue has: show "not found".
      try { return { name: "detail", id: decodeURIComponent(m[1]) }; } catch (e) {
        return { name: "detail", id: m[1] };
      }
    }
    return { name: "status" };
  }

  KW.go = function (url, replace) {
    if (replace) history.replaceState(null, "", url);
    else history.pushState(null, "", url);
    render();
    focusMain();
  };

  /* After moving to another page, start keyboard and screen reader
     users at its content, without scrolling. */
  function focusMain() {
    var m = document.getElementById("main");
    if (m) m.focus({ preventScroll: true });
  }

  /* safeNext keeps ?next= to a same-origin path. */
  function safeNext(raw) {
    // Control characters are dropped by URL parsing ("/\t/x" becomes
    // "//x"), so they could turn a path into another origin.
    if (!raw || raw.charAt(0) !== "/" || raw.charAt(1) === "/" ||
        raw.indexOf("\\") >= 0 || /[\u0000-\u001f\u007f]/.test(raw)) {
      return "/";
    }
    return raw;
  }

  function here() { return location.pathname + location.search; }

  /* authFailed handles 401/403 from any data call; true when it did. */
  KW.authFailed = function (res) {
    if (res.status === 403 && res.data && res.data.error === "https_required") {
      showHTTPS();
      return true;
    }
    if (res.status === 401 || res.status === 403) {
      var next = here();
      KW.live.stop();
      KW.go("/signin?reason=expired&next=" + encodeURIComponent(next), true);
      return true;
    }
    return false;
  };

  function stopView() {
    if (view && view.stop) view.stop();
    view = null;
  }

  function render() {
    var to = moved(location.pathname, location.search);
    if (to) history.replaceState(null, "", to);
    KW.rendered = here();
    stopView();
    var r = route(location.pathname);
    if (r.name === "detail") {
      KW.visits.opened(r.id);
      KW.notify.opened(r.id);
    }
    current = r.name;
    if (r.name === "signin") {
      shellEl = null;
      KW.notify.stop();
      KW.notify.signedOut();
      KW.appBadge(0);
      KW.stopDot();
      KW.live.stop();
      return showSignin();
    }
    var main = shell(r.name === "detail" ? "issues" : r.name);
    // Reads /api/status (every page then knows kwatch's own namespace,
    // api.js) every minute, for the dot on "kwatch".
    KW.startDot();
    var views = { issues: KW.issuesView, detail: KW.detailView,
      status: KW.statusView, kwatch: KW.kwatchView,
      activity: KW.activityView };
    view = views[r.name](main, r);
  }

  /* ---- shell ---- */
  function shell(current) {
    var app = document.getElementById("app");
    if (!shellEl) {
      app.textContent = "";
      app.appendChild(KW.clone("tpl-shell"));
      shellEl = app.firstElementChild;
      KW.$("[data-signout]", shellEl).addEventListener("click", signOut);
      KW.$$("[data-tz]", shellEl).forEach(function (b) {
        b.addEventListener("click", function () {
          KW.setUTC(b.dataset.tz === "utc");
          markTZ();
          // Redraw in the other zone where the reader is, not at the top.
          KW.pendingScroll = KW.scrollTop();
          render();
        });
      });
      markTZ();
      legendStates(shellEl);
      whoMenu(shellEl);
      KW.updateBadge(KW.badge);
      KW.notify.start(shellEl);
    }
    KW.setNav(current);
    var main = KW.$("#main", shellEl);
    main.textContent = "";
    window.scrollTo(0, 0);
    return main;
  }

  /* legendStates lists the states in the help, in KW.STATE's words. */
  function legendStates(root) {
    var ul = KW.$("[data-legend-states]", root);
    ["open", "settling", "flapping", "recovering", "resolved"].forEach(function (k) {
      var li = KW.el("li");
      li.appendChild(KW.el("b", "", KW.STATE[k].label));
      li.appendChild(document.createTextNode(": " + KW.STATE[k].title));
      ul.appendChild(li);
    });
  }

  /* whoMenu: under 700px wide, notifications, times and sign out fold
     behind one "Settings" button (plain.css). */
  function whoMenu(root) {
    var b = KW.$("[data-who-toggle]", root), side = KW.$(".side", root);
    b.addEventListener("click", function () {
      var open = side.classList.toggle("who-open");
      b.setAttribute("aria-expanded", open ? "true" : "false");
    });
  }

  function markTZ() {
    KW.$$("[data-tz]", shellEl).forEach(function (b) {
      b.setAttribute("aria-pressed", (b.dataset.tz === "utc") === KW.utc ? "true" : "false");
    });
  }

  KW.setNav = function (current) {
    if (!shellEl) return;
    KW.$$("[data-nav]", shellEl).forEach(function (a) {
      if (a.dataset.nav === current) a.setAttribute("aria-current", "page");
      else a.removeAttribute("aria-current");
    });
  };

  function signOut() {
    // Pushes stop first: signing out needs the session.
    // Whatever fails on the way, the user ends on the sign-in page.
    function go() { KW.go("/signin", true); }
    var out;
    try { out = Promise.resolve(KW.notify.signOut()); } catch (e) { out = Promise.resolve(); }
    out.then(null, function () { /* push unsubscribe failed */ })
      .then(function () { return KW.api("/logout", { method: "POST" }); })
      .then(null, function () { /* sign-out request failed */ })
      .then(go);
  }

  /* ---- sign in ---- */
  function showSignin() {
    // The cluster's name is for signed-in readers only: it may say
    // more than the address does (see KW.clusterTitle).
    KW.cluster = null;
    KW.title("Sign in");
    var app = document.getElementById("app");
    app.textContent = "";
    app.appendChild(KW.clone("tpl-signin"));
    var params = new URLSearchParams(location.search);
    var next = safeNext(params.get("next"));
    var reason = params.get("reason");
    KW.$("[data-restarted]", app).hidden = reason !== "restarted";
    KW.$("[data-expired]", app).hidden = reason !== "expired";
    var form = KW.$("form", app);
    var input = KW.$("#pw", app);
    var err = KW.$("[data-error]", app);
    var submit = KW.$("[data-submit]", app);
    var eye = KW.$("[data-eye]", app);
    KW.wireCopy(app);
    eye.addEventListener("click", function () {
      var show = input.type === "password";
      input.type = show ? "text" : "password";
      var label = show ? "Hide password" : "Show password";
      eye.querySelector("use").setAttribute("href", show ? "#i-eye-off" : "#i-eye");
      eye.setAttribute("aria-label", label);
      eye.setAttribute("title", label);
      eye.setAttribute("aria-pressed", show ? "true" : "false");
    });
    function fail(text, clear) {
      err.hidden = false;
      err.textContent = text;
      if (clear) input.value = "";
      input.setAttribute("aria-invalid", "true");
      input.focus();
    }
    function countdown(secs) {
      submit.disabled = true;
      var t = window.setInterval(function () {
        secs -= 1;
        if (secs <= 0 || !document.body.contains(submit)) {
          window.clearInterval(t);
          submit.disabled = false;
          submit.textContent = "Sign in";
          return;
        }
        submit.textContent = "Try again in " + secs + " s";
      }, 1000);
      submit.textContent = "Try again in " + secs + " s";
    }
    form.addEventListener("submit", function (e) {
      e.preventDefault();
      if (submit.disabled) return;
      submit.disabled = true;
      submit.textContent = "Signing in...";
      KW.api("/login", { method: "POST", body: { password: input.value } })
        .then(function (res) {
          submit.disabled = false;
          submit.textContent = "Sign in";
          if (res.status === 204) return KW.go(next, true);
          if (res.status === 403 && res.data && res.data.error === "https_required") {
            return showHTTPS();
          }
          if (res.status === 401) return fail("That password isn't right. Check it and try again.", true);
          if (res.status === 429) {
            fail("Too many tries. Wait a minute, then try again.");
            return countdown(res.retryAfter || 60);
          }
          if (res.status === 0) return fail("Can't reach kwatch. Check your connection and try again.");
          fail("Something went wrong inside kwatch. Try again.");
        });
    });
    input.focus();
  }

  function showHTTPS() {
    stopView();
    shellEl = null;
    KW.notify.stop();
    KW.stopDot();
    KW.live.stop();
    KW.title("Use https");
    var app = document.getElementById("app");
    app.textContent = "";
    app.appendChild(KW.clone("tpl-https"));
    var link = KW.$("[data-https-link]", app);
    var url = "https://" + location.host + "/";
    link.href = url;
    link.textContent = url;
  }

  /* ---- boot ---- */
  function boot() {
    document.addEventListener("click", function (e) {
      var a = e.target.closest && e.target.closest("a[data-link]");
      if (!a || e.defaultPrevented || e.button !== 0 || e.metaKey ||
          e.ctrlKey || e.shiftKey || e.altKey) {
        return;
      }
      e.preventDefault();
      KW.go(a.getAttribute("href"));
    });
    // A fragment link ("Skip to content") also fires popstate; only a
    // change of page re-renders. A view with pop() may take the change
    // itself (the Issues table opening or closing its preview).
    window.addEventListener("popstate", function () {
      if (here() !== KW.rendered && !(view && view.pop && view.pop())) render();
    });
    /* Keys: "/" jumps to search; j / k move down and up the list, or to
       the next and previous issue on an issue page. */
    document.addEventListener("keydown", function (e) {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      var t = e.target;
      if (t && (t.isContentEditable || /^(INPUT|SELECT|TEXTAREA)$/.test(t.tagName))) return;
      if (e.key === "/") {
        var q = document.querySelector('[data-f="q"]');
        if (!q || q.offsetParent === null) return;
        e.preventDefault();
        q.focus();
        q.select();
        return;
      }
      if (e.key !== "j" && e.key !== "k") return;
      var step = e.key === "j" ? 1 : -1;
      var rows = KW.$$("#main [data-rowlink]");
      if (!rows.length) return;
      var at = rows.indexOf(document.activeElement);
      var next = rows[Math.max(0, Math.min(rows.length - 1, at < 0 ? 0 : at + step))];
      e.preventDefault();
      next.focus();
      next.scrollIntoView({ block: "nearest" });
    });
    window.setInterval(KW.tickTimes, 30000);
    // The service worker (sw.js, registered by push.js) stays: it shows
    // the notifications and opens kwatch when one is clicked. A worker
    // of an earlier release at the same scope is replaced by it.
    KW.api("/api/session").then(function (res) {
      if (res.status === 403 && res.data && res.data.error === "https_required") {
        return showHTTPS();
      }
      var signedIn = res.status === 204;
      // A cookie from before kwatch restarted: say so on the sign-in page.
      var restarted = res.status === 200 && !!res.data && res.data.reason === "restarted";
      if (location.pathname === "/signin") {
        var q = new URLSearchParams(location.search);
        if (signedIn) return KW.go(safeNext(q.get("next")), true);
        if (restarted) {
          q.set("reason", "restarted");
          history.replaceState(null, "", "/signin?" + q.toString());
        }
        return render();
      }
      if (!signedIn && res.status !== 0) {
        return KW.go("/signin?" + (restarted ? "reason=restarted&" : "") +
          "next=" + encodeURIComponent(here()), true);
      }
      render();
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", boot);
  } else {
    boot();
  }
})();
