// SchalekPage service worker.
//
// Two caches, two very different jobs:
//
//   * STATIC_CACHE holds the CSS, fonts, icons and manifest — things that are
//     identical for every user and change only on deploy. They are precached
//     and then served stale-while-revalidate so repeat loads paint instantly.
//
//   * PAGES_CACHE holds the authenticated HTML of the primary destinations.
//     This is the part that makes a click feel like an app: the next page is
//     already on the device, so it paints without crossing the network. It is
//     deliberately time-boxed — a page is served without revalidation for
//     FRESH_MS and served-while-revalidating for at most MAX_STALE_MS — and it
//     is wiped the moment the session ends (logout, or any response that
//     redirects an authenticated page to the login screen), so one student's
//     grades never outlive their sign-in in Cache Storage.
//
// POST /logout is intercepted too: the browser routes form navigations
// through the service worker, so that is the earliest reliable point to drop
// the page cache.

const VERSION = "sp-v2";
const STATIC_CACHE = VERSION + "-static";
const PAGES_CACHE = VERSION + "-pages";
const OFFLINE_URL = "/static/offline.html";

// A page younger than FRESH_MS is served as-is; between FRESH_MS and
// MAX_STALE_MS it is served immediately and refreshed in the background;
// older than MAX_STALE_MS it is considered too stale to trust and the
// navigation waits on the network (falling back to the stale copy offline).
const FRESH_MS = 30 * 1000;
const MAX_STALE_MS = 10 * 60 * 1000;

// Response header we stamp cached pages with, so age survives a service
// worker restart (unlike an in-memory map).
const STAMP_HEADER = "x-sp-cached-at";

// Requests carrying this header are cache warm-ups triggered by the page; the
// service worker fetches them and stores the result, then answers 204.
const PREFETCH_HEADER = "x-sp-prefetch";

// The authenticated destinations that may be cached. Anything not listed
// (the login and 2FA screens, logout, OAuth, the MCP endpoint, the
// notification-history fragment) always goes straight to the network.
const PAGE_EXACT = ["/dashboard", "/grades", "/homework", "/exams", "/messages"];
const PAGE_PREFIX = ["/timetable/", "/lunches/", "/substitutions/"];

const PRECACHE_URLS = [
    "/static/css/style.css",
    "/static/css/main.css",
    "/static/manifest.webmanifest",
    "/static/icons/icon-192.png",
    "/static/icons/icon-512.png",
    "/static/vendor/fontawesome/css/all.min.css",
    "/static/vendor/fontawesome/webfonts/fa-solid-900.woff2",
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
                keys.filter((key) => key.startsWith("sp-") && key !== STATIC_CACHE && key !== PAGES_CACHE)
                    .map((key) => caches.delete(key))
            ))
            .then(() => self.clients.claim())
    );
});

self.addEventListener("fetch", (event) => {
    const req = event.request;
    const url = new URL(req.url);
    if (url.origin !== self.location.origin) return;

    // Session boundaries are POSTs; the GET-only fast path below would miss
    // them. Clearing on login/2FA (not just logout) matters because a page
    // cache left over from a previous student must never be served to the
    // next one on a shared device.
    if (req.method === "POST") {
        if (url.pathname === "/logout" || url.pathname === "/login" || url.pathname === "/two_factor") {
            event.respondWith(
                fetch(req).then((res) => {
                    void clearPages();
                    return res;
                })
            );
        }
        return;
    }
    if (req.method !== "GET") return;

    if (req.mode === "navigate") {
        event.respondWith(handleNavigation(event));
        return;
    }

    if (isCacheablePage(url) && req.headers.get(PREFETCH_HEADER) === "1") {
        event.respondWith(handlePrefetch(req));
        return;
    }

    if (url.pathname.startsWith("/static/")) {
        event.respondWith(handleStatic(req));
        return;
    }
});

// --- navigations ---------------------------------------------------------

async function handleNavigation(event) {
    const req = event.request;
    const url = new URL(req.url);

    // Landing on the login/2FA screen means there is no session: drop the
    // page cache so it cannot survive into the next sign-in. This also covers
    // a session that expired server-side, where an authenticated page was
    // redirected to "/".
    if (url.pathname === "/" || url.pathname === "/two_factor") {
        void clearPages();
        return networkOrOffline(req);
    }

    if (!isCacheablePage(url)) {
        return networkOrOffline(req);
    }

    const cache = await caches.open(PAGES_CACHE);
    const cached = await cache.match(req);
    const age = cached ? ageOf(cached) : Infinity;

    if (cached && age <= MAX_STALE_MS) {
        if (age > FRESH_MS) {
            schedule(event, revalidate(req));
        }
        return cached;
    }

    try {
        const res = await fetch(req);
        try {
            await maybeCachePage(req, res.clone());
        } catch (err) {
            /* caching is best-effort: still return the live response */
        }
        return res;
    } catch (err) {
        if (cached) return cached;
        return offlineFallback();
    }
}

// revalidate refreshes a cached page in the background so the *next*
// navigation is current. Failures are swallowed: a stale page is still better
// than no page.
async function revalidate(req) {
    try {
        const res = await fetch(req);
        await maybeCachePage(req, res);
    } catch (err) {
        /* offline — keep serving the copy we have */
    }
}

// maybeCachePage stores a page response, unless it is a login/2FA screen or
// an error — in which case the session has ended and the page cache is
// dropped instead.
async function maybeCachePage(req, res) {
    const finalURL = new URL(res.url);
    if (finalURL.pathname === "/" || finalURL.pathname === "/two_factor") {
        await clearPages();
        return;
    }
    if (!res.ok) return;
    if (!isCacheablePage(new URL(req.url))) return;

    const cache = await caches.open(PAGES_CACHE);
    const body = await res.blob();
    const headers = new Headers(res.headers);
    headers.set(STAMP_HEADER, String(Date.now()));
    await cache.put(req, new Response(body, {
        status: res.status,
        statusText: res.statusText,
        headers: headers,
    }));
}

// --- prefetch ------------------------------------------------------------

async function handlePrefetch(req) {
    try {
        const cache = await caches.open(PAGES_CACHE);
        const cached = await cache.match(req);
        // A still-fresh copy is already what the next click would get, so do
        // not spend an EduPage round trip on it. This keeps the idle warm-up
        // from re-rendering every destination on every navigation.
        if (!cached || ageOf(cached) > FRESH_MS) {
            const res = await fetch(req);
            await maybeCachePage(req, res);
        }
    } catch (err) {
        /* best-effort: a failed warm-up just means the next click hits the network */
    }
    return new Response(null, { status: 204 });
}

// --- static assets -------------------------------------------------------

async function handleStatic(req) {
    const cache = await caches.open(STATIC_CACHE);
    const cached = await cache.match(req);
    const network = fetch(req)
        .then((res) => {
            if (res.ok) cache.put(req, res.clone()).catch(() => {});
            return res;
        })
        .catch(() => cached);
    return cached || network;
}

// --- helpers -------------------------------------------------------------

function isCacheablePage(url) {
    const path = url.pathname;
    return PAGE_EXACT.indexOf(path) !== -1 ||
        PAGE_PREFIX.some((prefix) => path === prefix || path.startsWith(prefix));
}

function ageOf(cached) {
    const stamp = parseInt(cached.headers.get(STAMP_HEADER) || "", 10);
    return Number.isFinite(stamp) ? Date.now() - stamp : Infinity;
}

async function networkOrOffline(req) {
    try {
        return await fetch(req);
    } catch (err) {
        return offlineFallback();
    }
}

async function offlineFallback() {
    const offline = await caches.match(OFFLINE_URL);
    return offline || Response.error();
}

function schedule(event, promise) {
    try {
        event.waitUntil(promise);
    } catch (err) {
        /* Event no longer extendable; the promise still runs fire-and-forget. */
    }
}

async function clearPages() {
    try {
        await caches.delete(PAGES_CACHE);
    } catch (err) {
        /* nothing we can do; the old cache is at worst as stale as before */
    }
}
