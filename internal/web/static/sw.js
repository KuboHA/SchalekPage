// SchalekPage service worker.
//
// The app is server-rendered and every page carries a student's grades and
// timetable, so we deliberately do NOT cache HTML responses — that would
// mean silently showing stale grades, or leaving personal data sitting in
// the cache after the user logs out. What we do cache is the static shell
// (CSS, fonts icons, the manifest) so repeat loads and the standalone PWA
// window paint instantly, plus a tiny offline page for when navigation
// fails outright.
const CACHE_VERSION = "sp-v1";
const STATIC_CACHE = CACHE_VERSION + "-static";
const OFFLINE_URL = "/static/offline.html";

const PRECACHE_URLS = [
    "/static/css/style.css",
    "/static/css/main.css",
    "/static/manifest.webmanifest",
    "/static/icons/icon-192.png",
    "/static/icons/icon-512.png",
    OFFLINE_URL,
];

self.addEventListener("install", (event) => {
    event.waitUntil(
        caches.open(STATIC_CACHE)
            .then((cache) => cache.addAll(PRECACHE_URLS))
            .then(() => self.skipWaiting())
    );
});

self.addEventListener("activate", (event) => {
    event.waitUntil(
        caches.keys()
            .then((keys) => Promise.all(
                keys.filter((key) => key.startsWith("sp-") && key !== STATIC_CACHE)
                    .map((key) => caches.delete(key))
            ))
            .then(() => self.clients.claim())
    );
});

self.addEventListener("fetch", (event) => {
    const req = event.request;
    if (req.method !== "GET") return;

    const url = new URL(req.url);
    if (url.origin !== self.location.origin) return;

    // Full-page navigations: go to the network so the data is always
    // fresh; only fall back to the offline page if the network is gone.
    if (req.mode === "navigate") {
        event.respondWith(
            fetch(req).catch(() => caches.match(OFFLINE_URL))
        );
        return;
    }

    // Static assets: stale-while-revalidate so they paint instantly but
    // still pick up updates in the background.
    if (url.pathname.startsWith("/static/")) {
        event.respondWith(
            caches.open(STATIC_CACHE).then((cache) =>
                cache.match(req).then((cached) => {
                    const network = fetch(req)
                        .then((res) => {
                            if (res.ok) cache.put(req, res.clone());
                            return res;
                        })
                        .catch(() => cached);
                    return cached || network;
                })
            )
        );
    }
});
