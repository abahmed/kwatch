/* kwatch dashboard: the service worker. It shows the notifications
   kwatch pushes (push_run.go), with or without a kwatch tab open, and
   opens the issue when one is clicked. Every notification stays until
   it is closed: on some computers (macOS among them) a notification
   that leaves on its own is never shown. It caches nothing. */
"use strict";

self.addEventListener("install", function () { self.skipWaiting(); });
self.addEventListener("activate", function (e) {
  e.waitUntil(self.clients.claim());
});

/* safeURL keeps a link inside the dashboard. */
function safeURL(u) {
  return typeof u === "string" && u.charAt(0) === "/" &&
    u.charAt(1) !== "/" && u.charAt(1) !== "\\" ? u : "/";
}

self.addEventListener("push", function (e) {
  var m = {};
  try { m = (e.data && e.data.json()) || {}; } catch (err) { m = {}; }
  var tag = typeof m.tag === "string" && m.tag ? m.tag : "kwatch";
  e.waitUntil(self.registration.showNotification(
    typeof m.title === "string" && m.title ? m.title : "kwatch", {
      body: typeof m.body === "string" ? m.body : "",
      tag: tag, renotify: true, requireInteraction: true,
      data: { url: safeURL(m.url) }
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

/* A click opens the issue: in a kwatch tab already open (which moves
   there itself, notify.js), else in a new one. */
self.addEventListener("notificationclick", function (e) {
  e.notification.close();
  var url = safeURL(e.notification.data && e.notification.data.url);
  e.waitUntil(self.clients.matchAll({ type: "window",
    includeUncontrolled: true }).then(function (tabs) {
    for (var i = 0; i < tabs.length; i++) {
      if ("focus" in tabs[i]) {
        tabs[i].postMessage({ kwatch: "open", url: url });
        return tabs[i].focus();
      }
    }
    return self.clients.openWindow(url);
  }));
});
