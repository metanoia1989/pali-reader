// Service worker: the app shell, and nothing else.
//
// What this caches is the built frontend — index.html and the hashed bundles
// under /assets, which are immutable by construction. It deliberately does NOT
// touch /api: those responses are per-reader (picks, notes, translations) and a
// stale one would show a reader their own work from a previous session, or
// someone else's. The app's data layer already has its own cache in Redis on
// the server; a second, dumber one in the browser would only cause disagreement.
//
// The corpus is large and arrives in windows, so there is no attempt to make the
// reader work offline. What offline gives is the shell: the page still opens and
// says something useful instead of the browser's error page.

const CACHE = 'pali-shell-v1'
const SHELL = ['/', '/index.html', '/manifest.json', '/favicon.svg', '/icon-192.png']

self.addEventListener('install', (e) => {
  e.waitUntil(
    caches
      .open(CACHE)
      // A single missing file must not fail the whole install.
      .then((c) => Promise.allSettled(SHELL.map((u) => c.add(u))))
      .then(() => self.skipWaiting()),
  )
})

self.addEventListener('activate', (e) => {
  e.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  )
})

self.addEventListener('fetch', (e) => {
  const { request } = e
  if (request.method !== 'GET') return
  const url = new URL(request.url)
  if (url.origin !== self.location.origin) return
  // Never the API.
  if (url.pathname.startsWith('/api/')) return

  // Hashed assets and icons: serve from cache, they cannot change under a name.
  if (url.pathname.startsWith('/assets/') || /\.(png|svg|webmanifest)$/.test(url.pathname)) {
    e.respondWith(
      caches.match(request).then(
        (hit) =>
          hit ||
          fetch(request).then((res) => {
            const copy = res.clone()
            caches.open(CACHE).then((c) => c.put(request, copy))
            return res
          }),
      ),
    )
    return
  }

  // Everything else, index.html included: the network is the truth, because a
  // new deploy means new bundle names in the HTML. Fall back to the cached shell
  // when it is unreachable, which is the offline case.
  e.respondWith(
    fetch(request)
      .then((res) => {
        const copy = res.clone()
        caches.open(CACHE).then((c) => c.put(request, copy))
        return res
      })
      .catch(() => caches.match(request).then((hit) => hit || caches.match('/index.html'))),
  )
})
