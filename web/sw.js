// Minimal service worker: makes Hiveloop installable and shows notifications.
// No caching (testing phase): every request goes to the network; only page loads
// get a tiny offline message.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (e) => e.waitUntil(self.clients.claim()));
self.addEventListener("fetch", (e) => {
  if (e.request.mode !== "navigate") return; // API, SSE, assets: untouched
  e.respondWith(
    fetch(e.request).catch(() => new Response(
      '<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Hiveloop</title>' +
      '<body style="font-family:system-ui;background:#0c0c0b;color:#f2f0e9;display:grid;place-items:center;min-height:100vh;margin:0;text-align:center">' +
      '<div><p style="font:italic 56px Georgia,serif;margin:0">hiveloop.</p><p>Sei offline. Riprova quando torni connesso.<br><small>You are offline.</small></p></div>',
      { headers: { "Content-Type": "text/html; charset=utf-8" } }))
  );
});

// Push: private messages, mentions, volunteer reminders. Skipped while the app is open
// and in front: it already shows the same thing.
self.addEventListener("push", (e) => {
  let n = {};
  try { n = e.data ? e.data.json() : {}; } catch { n = { title: "Hiveloop", body: e.data ? e.data.text() : "" }; }
  e.waitUntil((async () => {
    const wins = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
    if (n.tag !== "test" && wins.some((c) => c.visibilityState === "visible" && c.focused)) return;
    await self.registration.showNotification(n.title || "Hiveloop", {
      body: n.body || "", tag: n.tag || undefined, renotify: !!n.tag,
      icon: "/icon-192.png", badge: "/icon-192.png", data: { url: n.url || "/" },
    });
  })());
});

self.addEventListener("notificationclick", (e) => {
  e.notification.close();
  const url = new URL(e.notification.data?.url || "/", self.location.origin).href;
  e.waitUntil((async () => {
    const wins = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
    const win = wins.find((c) => new URL(c.url).origin === self.location.origin);
    if (win) {
      await win.focus();
      return win.navigate(url).catch(() => win.postMessage({ type: "open", url }));
    }
    return self.clients.openWindow(url);
  })());
});
