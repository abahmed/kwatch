/* kwatch dashboard: Web Push, so notifications arrive with no kwatch
   tab open. The browser subscribes with kwatch's key (GET /api/push)
   and hands the subscription to kwatch (POST /api/push); kwatch then
   sends every change itself (push_run.go) and the service worker
   (sw.js) shows it. Every notification stays until closed: on some
   computers one that leaves on its own is never shown. Without a
   service worker or push service, notify.js shows them while a tab is
   open. */
(function () {
  "use strict";
  var KW = window.KW;
  var reg = null;
  var state = { on: false, endpoint: "", failing: "" };

  function supported() {
    return "serviceWorker" in navigator && "PushManager" in window &&
      window.isSecureContext !== false;
  }

  /* worker registers sw.js once and resolves with its registration,
     or null when there is none. */
  function worker() {
    if (!reg) {
      reg = !("serviceWorker" in navigator) ? Promise.resolve(null) :
        navigator.serviceWorker.register("/sw.js", { scope: "/" })
          .then(function () { return navigator.serviceWorker.ready; },
            function () { return null; });
    }
    return reg;
  }

  /* bytes reads base64url into the Uint8Array subscribe takes. */
  function bytes(s) {
    var b = atob(s.replace(/-/g, "+").replace(/_/g, "/") +
      "===".slice((s.length + 3) % 4));
    var out = new Uint8Array(b.length);
    for (var i = 0; i < b.length; i++) out[i] = b.charCodeAt(i);
    return out;
  }

  /* sameKey: the subscription was made with key. After kwatch made a
     new key it is useless and is replaced. */
  function sameKey(sub, key) {
    var have = sub.options && sub.options.applicationServerKey;
    if (!have) return false;
    var a = new Uint8Array(have), b = bytes(key);
    if (a.length !== b.length) return false;
    for (var i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
    return true;
  }

  /* subscribe signs this browser up, or updates what it is told
     (prefs: min, resolved). fresh drops the browser's subscription
     first: kwatch forgot it, so its push service had ended it. It
     resolves true when kwatch will push. */
  function subscribe(prefs, fresh) {
    if (!supported()) return Promise.resolve(false);
    return worker().then(function (r) {
      if (!r) return false;
      return KW.api("/api/push").then(function (res) {
        if (res.status !== 200 || !res.data || !res.data.publicKey) {
          return false;
        }
        var key = res.data.publicKey;
        return r.pushManager.getSubscription().then(function (sub) {
          if (sub && !fresh && sameKey(sub, key)) return sub;
          return Promise.resolve(sub && sub.unsubscribe()).then(function () {
            return r.pushManager.subscribe({ userVisibleOnly: true,
              applicationServerKey: bytes(key) });
          });
        }).then(function (sub) {
          return KW.api("/api/push", { method: "POST", body: {
            subscription: sub.toJSON(), min: prefs.min,
            resolved: prefs.resolved } }).then(function (res) {
            if (res.status !== 204) return false;
            state = { on: true, endpoint: sub.endpoint, failing: "" };
            return true;
          });
        });
      });
    }).catch(function () { return false; }).then(function (ok) {
      if (!ok) state = { on: false, endpoint: "", failing: "" };
      return ok;
    });
  }

  /* unsubscribe stops pushes to this browser: notifications turned
     off, or signing out. */
  function unsubscribe() {
    var endpoint = state.endpoint;
    state = { on: false, endpoint: "", failing: "" };
    if (!supported()) return Promise.resolve();
    return navigator.serviceWorker.getRegistration("/").then(function (r) {
      return r ? r.pushManager.getSubscription() : null;
    }).then(function (sub) {
      var ep = (sub && sub.endpoint) || endpoint;
      var told = ep ? KW.api("/api/push/remove", { method: "POST",
        body: { endpoint: ep } }) : Promise.resolve();
      return told.then(function () { return sub && sub.unsubscribe(); });
    }).catch(function () { /* nothing to undo */ });
  }

  /* forget drops this browser's subscription without telling kwatch,
     which can't be told: the sign-in ended. Its push service then
     refuses kwatch's next message and kwatch forgets it too. */
  function forget() {
    state = { on: false, endpoint: "", failing: "" };
    if (!supported()) return;
    navigator.serviceWorker.getRegistration("/").then(function (r) {
      return r ? r.pushManager.getSubscription() : null;
    }).then(function (sub) {
      return sub && sub.unsubscribe();
    }).catch(function () { /* nothing to drop */ });
  }

  /* check asks how pushes to this browser go. It resolves "lost" when
     kwatch no longer knows it (a new password, an ended subscription,
     30 days unseen), so the caller subscribes again. The push address
     goes in the body: proxies log URLs. */
  function check() {
    if (!state.on) return Promise.resolve("");
    return KW.api("/api/push/status", { method: "POST",
      body: { endpoint: state.endpoint } }).then(function (res) {
      if (res.status !== 200 || !res.data) return "";
      if (!res.data.subscribed) {
        state.on = false;
        return "lost";
      }
      state.failing = res.data.failing || "";
      return "";
    });
  }

  /* test sends one notification through kwatch now; it resolves with
     "" or why it failed. */
  function test() {
    return KW.api("/api/push/test", { method: "POST",
      body: { endpoint: state.endpoint } }).then(function (res) {
      if (res.status === 204) return "";
      return (res.data && res.data.error) || "kwatch couldn't send it.";
    });
  }

  /* show shows one notification from this tab: through the service
     worker when there is one, so it behaves like a pushed one. */
  function show(n) {
    var opts = { body: n.body, tag: n.tag, renotify: true,
      requireInteraction: true, data: { url: n.url } };
    return worker().then(function (r) {
      if (r) return r.showNotification(n.title, opts);
      var note = new window.Notification(n.title, opts);
      note.onclick = function () {
        window.focus();
        open(n.url);
        note.close();
      };
    }).catch(function () { /* e.g. permission withdrawn meanwhile */ });
  }

  /* close closes the notifications of these issues: they resolved, or
     were opened here. */
  function close(ids) {
    if (!ids.length || !("serviceWorker" in navigator)) return;
    navigator.serviceWorker.getRegistration("/").then(function (r) {
      return r ? r.getNotifications() : [];
    }).then(function (list) {
      list.forEach(function (n) {
        if (ids.indexOf(n.tag) >= 0) n.close();
      });
    }).catch(function () { /* nothing open */ });
  }

  function open(url) {
    if (typeof url !== "string" || url.charAt(0) !== "/" ||
        url.charAt(1) === "/" || url.charAt(1) === "\\") return;
    if (KW.go && location.pathname !== url) KW.go(url);
  }

  // A click on a notification while a tab is open moves that tab.
  if ("serviceWorker" in navigator) {
    navigator.serviceWorker.addEventListener("message", function (e) {
      if (e.data && e.data.kwatch === "open") open(e.data.url);
    });
  }

  KW.push = { supported: supported, subscribe: subscribe,
    unsubscribe: unsubscribe, forget: forget, check: check, test: test, show: show,
    close: close,
    /* active: kwatch pushes to this browser and the last send went. */
    active: function () { return state.on && !state.failing; },
    on: function () { return state.on; },
    failing: function () { return state.failing; } };
})();
