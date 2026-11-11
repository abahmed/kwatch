/* kwatch dashboard: the service worker. It shows the notifications
   kwatch pushes (push_run.go), with or without a kwatch tab open, and
   opens the issue when one is clicked. A new version takes over at
   once (skipWaiting, clients.claim): nobody has to subscribe again. Every notification stays until
   it is closed: on some computers (macOS among them) a notification
   that leaves on its own is never shown. It caches nothing and handles
   no requests: every page and file comes from kwatch. */
"use strict";

/* ICON is the kwatch logo beside each notification (a bitmap: not
   every system draws SVG there); the server fills in its address. */
var ICON = "{{asset:icon-192.png}}";

self.addEventListener("install", function () { self.skipWaiting(); });
self.addEventListener("activate", function (e) {
  e.waitUntil(self.clients.claim());
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

/* address: the notification's page as a full address inside the
   dashboard: the worker's scope (the dashboard's root, so a dashboard
   served under a path prefix keeps it) plus the page. */
function address(n) {
  var path = safeURL(n && n.data && n.data.url);
  return new URL(path.slice(1), self.registration.scope).href;
}

/* A click brings kwatch to the front and opens the page of the
   notification (the issue; the Overview for the test one): a kwatch
   tab already on it, else another kwatch tab, which moves there itself
   (push.js), else a new tab. A tab that can't be brought forward gets
   a new one too. */
self.addEventListener("notificationclick", function (e) {
  e.notification.close();
  var url = address(e.notification);
  e.waitUntil(self.clients.matchAll({ type: "window",
    includeUncontrolled: true }).then(function (tabs) {
    var on = tabs.filter(function (t) {
      return t.url.split("#")[0] === url && "focus" in t;
    });
    var tab = on[0] || pick(tabs);
    if (!tab) return self.clients.openWindow(url);
    return tab.focus().then(function (t) {
      var to = t || tab;
      if (to.url.split("#")[0] !== url) {
        var u = new URL(url);
        to.postMessage({ kwatch: "open", url: u.pathname + u.search });
      }
      return to;
    }).catch(function () { return self.clients.openWindow(url); });
  }));
});
