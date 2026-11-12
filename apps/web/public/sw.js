/* TinyNest Phase 0: only cache PUBLIC shell and icons; never cache /api or sensitive data. */
const CACHE = 'tinynest-shell-phase0-v1';
const SHELL = ['/', '/index.html', '/manifest.webmanifest', '/icons/icon-192.png', '/icons/icon-512.png'];
self.addEventListener('install', (event) => {
  event.waitUntil(caches.open(CACHE).then((cache) => cache.addAll(SHELL)).then(() => self.skipWaiting()));
});
self.addEventListener('activate', (event) => {
  event.waitUntil(caches.keys().then((keys) => Promise.all(keys.filter((k) => k.startsWith('tinynest-shell-') && k !== CACHE).map((k) => caches.delete(k)))).then(() => self.clients.claim()));
});
self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin || url.pathname.startsWith('/api/')) return;
  if (req.mode === 'navigate') {
    event.respondWith(fetch(req).catch(() => caches.match('/index.html')));
    return;
  }
  if (/^\/(assets\/|icons\/|manifest\.webmanifest$|favicon\.ico$)/.test(url.pathname)) {
    event.respondWith(caches.match(req).then((hit) => hit || fetch(req).then((response) => {
      if (response.ok && response.type === 'basic') {
        const copy = response.clone(); caches.open(CACHE).then((cache) => cache.put(req, copy));
      }
      return response;
    })));
  }
});
