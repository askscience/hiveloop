// Minimal service worker: makes Hiveloop installable. No caching (testing phase):
// every request goes to the network; only page loads get a tiny offline message.
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
