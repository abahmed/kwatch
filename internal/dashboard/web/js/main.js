/* kwatch dashboard: router, shell, sign in and the live refresh loop. */
(function () {
  "use strict";
  var KW = window.KW;
  var view = null;
  var shellEl = null;
  KW.lastList = { active: "/", resolved: "/resolved" };

  /* ---- routing ---- */
  function route(path) {
    if (path === "/signin") return { name: "signin" };
    if (path === "/") return { name: "active" };
    if (path === "/resolved") return { name: "resolved" };
    if (path === "/status") return { name: "status" };
    var m = /^\/issues\/([^/]+)$/.exec(path);
    if (m) return { name: "detail", id: decodeURIComponent(m[1]) };
    return { name: "active" };
  }

  KW.go = function (url, replace) {
    if (replace) history.replaceState(null, "", url);
    else history.pushState(null, "", url);
    render();
  };

  /* safeNext keeps ?next= to a same-origin path. */
  function safeNext(raw) {
    if (!raw || raw.charAt(0) !== "/" || raw.charAt(1) === "/" ||
        raw.indexOf("\\") >= 0) {
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
    stopView();
    var r = route(location.pathname);
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
      KW.updateBadge(KW.badge);
    }
    KW.setNav(current);
    var main = KW.$("#main", shellEl);
    main.textContent = "";
    window.scrollTo(0, 0);
    return main;
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

  /* footer: "Live · Updates by itself · checked N s ago", or after a
     failure "Last updated N min ago" with Try now. */
  KW.footer = function (node, lastOk, failing, onTry) {
    node.hidden = !lastOk;
    if (!lastOk) return;
    node.textContent = "";
    var ago = Math.max(0, Math.round((Date.now() - lastOk) / 1000));
    if (failing) {
      node.appendChild(KW.el("span", "", "Last updated " +
        KW.rel(new Date(lastOk + (KW.now() - Date.now())).toISOString())));
      var b = KW.el("button", "btn btn-quiet btn-sm", "Try now");
      b.type = "button";
      b.addEventListener("click", onTry);
      node.appendChild(b);
      return;
    }
    node.appendChild(KW.el("span", "live", "Live"));
    node.appendChild(KW.el("span", "", "Updates by itself · checked " + ago + " s ago"));
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
    window.addEventListener("popstate", render);
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
