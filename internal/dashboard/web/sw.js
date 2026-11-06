/* kwatch dashboard: the service worker. It shows the notifications
   kwatch pushes (push_run.go), with or without a kwatch tab open, and
   opens the issue when one is clicked. Every notification stays until
   it is closed: on some computers (macOS among them) a notification
   that leaves on its own is never shown. It caches one thing: the
   "Can't reach kwatch" page (offline.html) and the two files it uses,
   shown when opening a page fails or the way in answers 502, 503 or
   504. Everything else always comes from kwatch. */
"use strict";

/* OFFLINE is the page and its files. The cache name carries the hashed
   stylesheet name and a hash of the page (static.go versionWorker), so
   any release that changes either changes this worker too: browsers
   install it and it keeps its own copy. */
var OFFLINE = ["/offline.html", "{{asset:app.css}}", "{{asset:favicon.svg}}"];
var CACHE = "kwatch-offline-" + OFFLINE[1] + "-{{version:offline.html}}";

/* ICON is the kwatch logo beside each notification (a bitmap: not
   every system draws SVG there); the server fills in its address. */
var ICON = "{{asset:icon-192.png}}";

self.addEventListener("install", function (e) {
  self.skipWaiting();
  // Best effort: without the copy, a failed page shows as before.
  e.waitUntil(caches.open(CACHE).then(function (c) {
    return c.addAll(OFFLINE);
  }).catch(function () {}));
});
self.addEventListener("activate", function (e) {
  e.waitUntil(caches.keys().then(function (keys) {
    return Promise.all(keys.filter(function (k) {
      return k.indexOf("kwatch-offline-") === 0 && k !== CACHE;
    }).map(function (k) { return caches.delete(k); }));
  }).then(function () { return self.clients.claim(); }));
});

function offline(res) {
  return caches.match("/offline.html", { cacheName: CACHE }).then(function (page) {
    return page || res || Response.error();
  });
}

/* Opening a page: from kwatch, else the "Can't reach kwatch" page. The
   page's two files come from the copy (their names never change).
   Every other request goes to kwatch untouched. */
self.addEventListener("fetch", function (e) {
  var req = e.request;
  if (req.method !== "GET") return;
  if (req.mode === "navigate") {
    e.respondWith(fetch(req).then(function (res) {
      return res.status >= 502 && res.status <= 504 ? offline(res) : res;
    }, function () { return offline(null); }));
    return;
  }
  var path = new URL(req.url).pathname;
  if (path !== OFFLINE[0] && OFFLINE.indexOf(path) > 0) {
    e.respondWith(caches.match(req, { cacheName: CACHE }).then(function (hit) {
      return hit || fetch(req);
    }));
  }
});

/* safeURL keeps a link inside the dashboard. */
function safeURL(u) {
  return typeof u === "string" && u.charAt(0) === "/" &&
    u.charAt(1) !== "/" && u.charAt(1) !== "\\" ? u : "/";
}

/* badge puts the urgent count on the installed app's icon; 0 clears
   it. Browsers without it ignore it. */
function badge(n) {
  var nav = self.navigator;
  if (typeof n !== "number" || !nav.setAppBadge) return Promise.resolve();
  return (n > 0 ? nav.setAppBadge(n) : nav.clearAppBadge()).catch(function () {});
}

self.addEventListener("push", function (e) {
  var m = {};
  try { m = (e.data && e.data.json()) || {}; } catch (err) { m = {}; }
  var tag = typeof m.tag === "string" && m.tag ? m.tag : "kwatch";
  e.waitUntil(badge(m.badge).then(function () {
    return self.registration.showNotification(
      typeof m.title === "string" && m.title ? m.title : "kwatch", {
        body: typeof m.body === "string" ? m.body : "",
        tag: tag, renotify: true, requireInteraction: true, icon: ICON,
        data: { url: safeURL(m.url) }
      });
  }));
});

/* The browser renewed the subscription (some do now and then): hand
   the new one to kwatch, which keeps this browser's settings. Without
   a sign-in it fails, and the next sign-in subscribes again. */
self.addEventListener("pushsubscriptionchange", function (e) {
  var old = e.oldSubscription;
  var key = old && old.options && old.options.applicationServerKey;
  e.waitUntil(Promise.resolve(e.newSubscription || (key &&
    self.registration.pushManager.subscribe({ userVisibleOnly: true,
      applicationServerKey: key }))).then(function (sub) {
    if (!sub) return null;
    return fetch("/api/push", { method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ subscription: sub.toJSON(),
        replaces: old ? old.endpoint : "" }) });
  }).catch(function () { /* the next sign-in subscribes again */ }));
});

/* pick: the kwatch tab to bring forward: the focused one, else a
   visible one, else any; null when none is open. */
function pick(tabs) {
  var best = null;
  for (var i = 0; i < tabs.length; i++) {
    var t = tabs[i];
    if (!("focus" in t)) continue;
    if (t.focused) return t;
    if (!best || (t.visibilityState === "visible" &&
        best.visibilityState !== "visible")) best = t;
  }
  return best;
}

/* A click brings kwatch to the front and opens the issue: in a kwatch
   tab already open (which moves there itself, push.js), else in a new
   one. A tab that can't be brought forward gets a new one too. */
self.addEventListener("notificationclick", function (e) {
  e.notification.close();
  var url = safeURL(e.notification.data && e.notification.data.url);
  e.waitUntil(self.clients.matchAll({ type: "window",
    includeUncontrolled: true }).then(function (tabs) {
    var tab = pick(tabs);
    if (!tab) return self.clients.openWindow(url);
    return tab.focus().then(function (t) {
      (t || tab).postMessage({ kwatch: "open", url: url });
    }, function () { return self.clients.openWindow(url); });
  }));
});
