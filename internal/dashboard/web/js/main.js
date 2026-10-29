/* kwatch dashboard: router, shell, sign in and the live refresh loop. */
(function () {
  "use strict";
  var KW = window.KW;
  var view = null;
  var shellEl = null;
  var current = "";
  KW.lastList = { active: "/", resolved: "/resolved" };
  /* Scroll positions of the lists, so coming back from an issue lands
     where you left instead of at the top. */
  KW.scrollMemo = {};
  KW.cameFrom = "";
  /* Issue IDs in the order each list shows them, for Previous / Next. */
  KW.listIds = { active: [], resolved: [] };

  /* ---- routing ---- */
  function route(path) {
    if (path === "/signin") return { name: "signin" };
    if (path === "/") return { name: "active" };
    if (path === "/resolved") return { name: "resolved" };
    if (path === "/status") return { name: "status" };
    var m = /^\/issues\/([^/]+)$/.exec(path);
    if (m) {
      // A malformed escape (%E0) is an ID no issue has: show "not found".
      try { return { name: "detail", id: decodeURIComponent(m[1]) }; } catch (e) {
        return { name: "detail", id: m[1] };
      }
    }
    return { name: "active" };
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
    KW.rendered = here();
    if (current === "active" || current === "resolved") {
      KW.scrollMemo[KW.lastList[current]] = window.scrollY;
    }
    stopView();
    var r = route(location.pathname);
    KW.cameFrom = current;
    current = r.name;
    if (r.name === "signin") {
      shellEl = null;
      return showSignin();
    }
    var main = shell(r.name === "detail" ? "active" : r.name);
    var views = { active: KW.listView, resolved: KW.listView,
      detail: KW.detailView, status: KW.statusView };
    view = views[r.name](main, r);
    if (r.name === "active" || r.name === "resolved") {
      KW.lastList[r.name] = here();
    }
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
          KW.pendingScroll = window.scrollY;
          render();
        });
      });
      markTZ();
      KW.updateBadge(KW.badge);
    }
    KW.setNav(current);
    var main = KW.$("#main", shellEl);
    main.textContent = "";
    window.scrollTo(0, 0);
    return main;
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
    KW.api("/logout", { method: "POST" }).then(function () {
      KW.go("/signin", true);
    });
  }

  /* ---- active count: nav badge, tab title, favicon ---- */
  KW.badge = null;
  KW.updateBadge = function (counts) {
    KW.badge = counts;
    if (!counts) return;
    var total = 0;
    Object.keys(counts).forEach(function (k) {
      if (k !== "silent") total += counts[k];
    });
    var urgent = (counts.page || 0) > 0;
    var b = shellEl && KW.$("[data-badge]", shellEl);
    if (b) {
      b.hidden = total === 0;
      b.textContent = total;
      b.className = "count" + (urgent ? " urgent" : "");
      b.setAttribute("aria-label", KW.plural(total, "active issue", "active issues"));
    }
    var fav = document.getElementById("favicon");
    if (!fav.dataset.plain) fav.dataset.plain = fav.getAttribute("href");
    fav.setAttribute("href", urgent ? fav.dataset.alert : fav.dataset.plain);
    KW.activeTotal = total;
  };
  KW.refreshBadge = function () {
    return KW.api("/api/incidents?state=active&limit=1").then(function (res) {
      if (res.status === 200 && res.data) KW.updateBadge(res.data.counts);
    });
  };
  KW.title = function (text) { document.title = text + " · kwatch"; };

  /* ---- live refresh ---- */
  /* poller runs load now and every `every` ms while the tab is visible,
     at once when it becomes visible again. load returns a promise of a
     delay override (ms) or nothing. */
  KW.poller = function (load, every) {
    var timer = 0, stopped = false;
    function schedule(ms) {
      window.clearTimeout(timer);
      if (!stopped && !document.hidden) timer = window.setTimeout(run, ms);
    }
    function run() {
      window.clearTimeout(timer);
      if (stopped) return;
      Promise.resolve(load()).then(function (delay) {
        schedule(delay || every);
      });
    }
    function onVisible() {
      if (document.hidden) window.clearTimeout(timer);
      else run();
    }
    document.addEventListener("visibilitychange", onVisible);
    run();
    return {
      now: run,
      stop: function () {
        stopped = true;
        window.clearTimeout(timer);
        document.removeEventListener("visibilitychange", onVisible);
      }
    };
  };

  /* footer: the freshness line at the top of a page. "Live · updated
     just now" while the page refreshes by itself, or after a failure
     "Last updated 4 min ago" with Try now. */
  KW.footer = function (node, lastOk, failing, onTry) {
    node.hidden = !lastOk;
    if (!lastOk) return;
    node.textContent = "";
    node.classList.toggle("stale", !!failing);
    var ago = Math.max(0, Math.round((Date.now() - lastOk) / 1000));
    if (failing) {
      node.appendChild(KW.el("span", "", "Last updated " +
        KW.rel(new Date(lastOk + (KW.now() - Date.now())).toISOString())));
      var b = KW.el("button", "btn btn-sm upd-try", "Try now");
      b.type = "button";
      b.addEventListener("click", onTry);
      node.appendChild(b);
      return;
    }
    var live = KW.el("span", "live", "Live");
    live.title = "This page updates by itself.";
    node.appendChild(live);
    node.appendChild(KW.el("span", "", "updated " +
      (ago < 5 ? "just now" : ago < 60 ? ago + " s ago" : Math.floor(ago / 60) + " min ago")));
  };

  /* ---- sign in ---- */
  function showSignin() {
    KW.title("Sign in");
    var app = document.getElementById("app");
    app.textContent = "";
    app.appendChild(KW.clone("tpl-signin"));
    var params = new URLSearchParams(location.search);
    var next = safeNext(params.get("next"));
    KW.$("[data-expired]", app).hidden = params.get("reason") !== "expired";
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
    function fail(text) {
      err.hidden = false;
      err.textContent = text;
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
          if (res.status === 401) return fail("That password isn't right. Check it and try again.");
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
    // change of page re-renders.
    window.addEventListener("popstate", function () {
      if (here() !== KW.rendered) render();
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
      var pageLink = document.querySelector(step > 0 ? "[data-next]" : "[data-prev]");
      if (pageLink) {
        if (pageLink.offsetParent !== null && pageLink.getAttribute("href")) KW.go(pageLink.getAttribute("href"));
        return;
      }
      var rows = KW.$$(".list a.issue");
      if (!rows.length) return;
      var at = rows.indexOf(document.activeElement);
      var next = rows[Math.max(0, Math.min(rows.length - 1, at < 0 ? 0 : at + step))];
      e.preventDefault();
      next.focus();
      next.scrollIntoView({ block: "nearest" });
    });
    window.setInterval(KW.tickTimes, 30000);
    KW.api("/api/session").then(function (res) {
      if (res.status === 403 && res.data && res.data.error === "https_required") {
        return showHTTPS();
      }
      var signedIn = res.status === 204;
      if (location.pathname === "/signin") {
        if (signedIn) return KW.go(safeNext(new URLSearchParams(location.search).get("next")), true);
        return render();
      }
      if (!signedIn && res.status !== 0) {
        return KW.go("/signin?next=" + encodeURIComponent(here()), true);
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
