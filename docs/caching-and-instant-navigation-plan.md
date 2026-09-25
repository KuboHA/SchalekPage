# Plan: make button clicks feel like an app

**Goal:** a navigation click should paint in well under 100 ms, on any device,
without showing stale grades or leaking one user's data into another's cache.

> **Status (implemented):** Phase 0 and Phase 2 are done. Phase 1
> (server-side stale-while-revalidate) was **deliberately dropped** — the goal
> is to keep the server stateless and cache only on the device. The notes
> about Phase 1 below are kept for the record.

## 1. Where the 500 ms actually goes

Every destination in the UI is a plain `<a href>` / form POST, so each click is
a **full page load**: browser → server → (render) → server → browser. The slow
part is not rendering (templates are parsed once and render in ~1 ms); it is the
synchronous calls the handler makes to EduPage before it can render anything.

Measured from a normal client network, `https://*.edupage.org` has a TTFB of
**210–250 ms per round trip**. Current critical paths:

| Route | Upstream work on the critical path | Serial RTTs |
| --- | --- | --- |
| `/dashboard` | `MyTimetable` (`GET /dashboard/eb.php?mode=ttday` → `POST /gcall`), `MealsFor` (`GET /menu/`) | **2** |
| `/timetable/{date}` | `MyTimetable` | **2** |
| `/substitutions/{date}` | `TimetableChanges` **and** `MissingTeachers`, each doing the *same* `POST /substitution/server/viewer.js`, sequentially | **2** (duplicate) |
| `/grades` | `GradesForTerm` (`POST /znamky/…`) | **1** |
| `/lunches/{date}` | `MealsFor` | **1** |
| `/homework`, `/exams`, `/messages` | none — answered from the login payload | 0 |

So the flagship pages cost ~500 ms of upstream latency *before any HTML is
produced*. That is exactly the reported delay.

Two independent fixes are needed:

1. **Stop paying the 2 × 250 ms on the critical path** → server-side cache with
   stale-while-revalidate.
2. **Stop doing a cold, white-flash, full reload for every click** → on-device
   (service worker) page cache + intent prefetch, so a click is served from the
   device.

Neither alone is enough: #1 makes the server answer in single-digit ms but the
browser still waits on a network round trip and a full parse/paint; #2 makes the
network hop disappear but only helps for pages whose HTML was already fetched.

---

## 2. Phase 0 — quick wins (no new architecture, ~1–2 h)

These are independent of any cache and should land first.

### 2.1 Fetch the substitution page once

`handleSubstitutions` (`internal/web/handlers.go`) calls `TimetableChanges(day)`
and then `MissingTeachers(day)`. Both call `substitutionHTML(day)`, i.e. the
identical `POST /substitution/server/viewer.js` is made twice, serially.

Replace with one method on the client, e.g.
`Client.SubstitutionDay(day) (changes []TimetableChange, missing []Teacher, err error)`,
that fetches the HTML once and derives both. Saves a full ~250 ms on the
substitutions page **and** removes a redundant EduPage request per view.

### 2.2 Instrument the upstream time

Add a `Server-Timing` response header (dev flag or always) with
`upstream;dur=<ms>, cache;hit|miss|stale, render;dur=<ms>`. This is how we prove
the cache works and catch regressions. Only a few lines in `render.go` / a small
`timing` helper on `Server`.

### 2.3 Take third-party CDNs off the critical path

`partials.html` loads Font Awesome from `cdnjs.cloudflare.com` and
`timetable.html` / `substitutions.html` load Alpine from `unpkg.com`. Each is a
separate DNS + TLS handshake on every navigation, and neither is covered by the
service worker (cross-origin).

- Self-host Font Awesome CSS + the woff2 files under `/static/` and add them to
  the SW precache.
- Alpine is only used for a loading spinner / page transition on two pages.
  Once navigation is instant (Phase 2) the spinner is a flash. Drop Alpine and
  the transition entirely; replace with a few lines of inline JS if any
  behaviour is still wanted.

---

## 3. Phase 1 — per-session upstream cache with stale-while-revalidate

This is the highest-value change. It lives in `internal/edupage` because the
cache key and lifetime belong next to the call that owns the data, and because a
`Client` is already **one per session** (`internal/web/session.go`), so entries
are naturally isolated per user and are dropped with the session on logout/reap.

### 3.1 Shape

New `internal/edupage/cache.go`:

```go
type cacheEntry struct {
    val        any
    freshUntil time.Time // before this: serve as-is, no refresh
    staleUntil time.Time // after freshUntil but before this: serve + refresh in background
    refreshing bool
    lastErr    error
}

// cached returns the entry for key, fetching it on a miss. On a stale hit it
// returns the old value immediately and refreshes in the background.
func (c *Client) cached(key string, fresh, stale time.Duration, fetch func() (any, error)) (any, error)
```

- Guarded by a `sync.Mutex` on the cache; `Client` is already called
  concurrently from `runConcurrently`, so this must be safe.
- **Single-flight**: hand-rolled (no new dependency — keep the "no third-party
  deps" promise) so two concurrent page loads for the same key issue one
  upstream request, and so `TimetableChanges` + `MissingTeachers` share one
  fetch even on a cold cache.
- **Errors are never cached.** A failed background refresh keeps the old value
  and clears `refreshing`.
- **Hard eviction**: entries unused beyond `staleUntil + grace` are dropped, and
  a per-session entry cap (e.g. 128) with simple LRU protects memory.
- Values are the already-parsed Go structs (`*Timetable`, `*Meals`, `[]Grade`, …),
  so repeated renders skip parsing as well.

### 3.2 Wrapping the read calls

| Method | Key | Fresh | Serve-stale window | Invalidate on |
| --- | --- | --- | --- | --- |
| `MyTimetable(day)` | `tt:YYYY-MM-DD` | 5 min (today) / 60 min (other days) | 60 min | — |
| `MealsFor(day)` | `meals:YYYY-MM-DD` | 60 s | 15 min | `ChooseMeal`, `SignOffMeal`, `RateMeal` |
| `GradesForTerm(year, term)` | `grades:YEAR:TERM` | 5 min | 24 h | — |
| `SubstitutionDay(day)` | `subst:YYYY-MM-DD` | 2 min | 30 min | — |
| `NotificationsSince(from)` | `notif:YYYY-MM-DD` | 5 min | 1 h | — |

Past dates (day < today) are effectively immutable: use a very long fresh window
(24 h) since timetable, meals and substitutions for a finished day do not change.
The dashboard's 14:30 rollover is already keyed by the target date, so the cache
key naturally flips with it.

Ordering deadlines in `MealsFor` are minute-sensitive, which is why the meal
window is only 60 s **and** every meal write deletes `meals:<day>` immediately
(write-through invalidation), so the user always sees the result of their own
order.

### 3.3 Warm the cache at login

The only remaining blocking latency after Phase 1 is the **first** dashboard
load after login or server restart. In `finishLoginRedirect`
(`internal/web/handlers_auth.go`), after the client is authenticated, spawn a
background goroutine that warms `MyTimetable(today)`, `MealsFor(today)` and
`NotificationsSince(-30d)`. By the time the browser follows the redirect the
app is usually already warm — even the first click is instant.

(Safe because the same `*edupage.Client` is already used concurrently by
`runConcurrently`; see the `Client` doc comment.)

### 3.4 Logout / reap

Nothing extra to do: the cache hangs off the `*Session`'s `Client`, and
`Store.Delete` / `reap` drop the whole session. No server-side persistence, so a
restart logs everyone out exactly as it does today.

### 3.5 Freshness UX

With SWR, the *first* click after the fresh window can show a value that is one
background-refresh old. That is acceptable for timetable/meals/substitutions
(minutes), and mitigated for grades by keeping the fresh window short (5 min)
and revalidating on page focus (`visibilitychange` → a small `fetch` to the
current route, which touches the cache). Do **not** cache
`/notifications/history` (already a fast fragment fetched on demand) and do not
cache anything in the MCP path.

---

## 4. Phase 2 — on-device cache + instant navigation (service worker)

The current `static/sw.js` deliberately does **not** cache HTML, because pages
contain grades/timetable and the cache could outlive a logout. Phase 2 adds an
HTML cache that is fast *and* privacy-safe, plus intent prefetching so clicks
are served locally.

### 4.1 HTML cache with an explicit freshness policy

Bump to `sp-v2`, add a `PAGES_CACHE`. In the `fetch` handler for
`req.mode === "navigate"`:

1. **Never cache** `/`, `/two_factor`, `/logout`, `/oauth/*`, `/mcp`,
   `/notifications/history`, `/static/*` (already handled) — anything not in the
   allow-list of GET page routes (`dashboard`, `timetable`, `lunches`, `grades`,
   `substitutions`, `homework`, `exams`, `messages`).
2. If a cached response exists and is younger than `FRESH_MS` (e.g. 60 s):
   return it immediately and revalidate in the background.
3. If cached but older than `MAX_STALE_MS` (e.g. 10 min): network-first, fall
   back to cache only when offline.
4. Otherwise (stale but within `MAX_STALE`): return cache instantly and
   revalidate in the background.
5. On any response, if the request was redirected to `/` (auth expiry) **or** the
   posted navigation target was `/logout`, wipe `PAGES_CACHE`. Also expose a
   `postMessage` so a page can ask the SW to clear on demand.

The cache is per-browser-profile which is the same boundary as the cookie jar, so
no cross-user leakage. The logout/expiry wipe plus the short `MAX_STALE` mean
personal data survives at most a few minutes after sign-out.

### 4.2 Prefetch on intent and on idle

- Add `data-prefetch` (or just use the existing `[data-nav-link]`) to primary nav
  links. On `pointerenter` / `touchstart`, issue
  `fetch(href, { credentials: "same-origin" })` with a marker header
  (`X-SP-Prefetch: 1`); the SW stores the response in `PAGES_CACHE`.
- After `load` + `requestIdleCallback`, prefetch the remaining primary routes so
  every tab in the bottom bar is warm before it is tapped. This is where the
  "app-like" feel comes from on mobile.
- Enable `navigationPreload` and add `@view-transition { navigation: auto }` to
  the stylesheet for cross-document cross-fades — purely progressive.

An uncached click then costs one network round trip to our own server (which
Phase 1 answers from cache in single-digit ms), not two EduPage round trips.

### 4.3 Update the offline story

With HTML cached, the main pages now work offline. Keep `offline.html` for
routes that were never visited; update its copy.

### 4.4 Explicitly out of scope

A JSON API + client-side router / SPA hydration. It would make transitions
sub-frame, but it is a large rewrite of every template and handler and is not
needed to hit the goal. Revisit only if view transitions prove insufficient.

---

## 5. Verification

- **Instrumentation**: `Server-Timing` (2.2) plus a temporary debug log of
  handler duration. Compare `time_total` for a route before/after with
  `curl -w`.
- **Targets**:
  - server answer for a cached route: **< 20 ms**;
  - SW-cached navigation (DevTools, offline or throttled): **< 100 ms** first
    paint;
  - uncached first navigation: ≤ ~300 ms thanks to prefetch + warm cache.
  - zero duplicate EduPage requests per page view (assert `SubstitutionDay`
    makes one call).
- **Tests** (Go, offline like the existing suite):
  - cache: fresh hit does not call fetch; stale hit returns old value and
    triggers exactly one refresh; concurrent misses share one fetch; errors are
    not cached; `ChooseMeal`/`SignOffMeal` invalidate the day's meals; past-date
    long TTL; entry cap.
  - `SubstitutionDay` returns both views from one HTML fetch (fake client).
  - handler test that `/substitutions/` issues one upstream request.
  - service worker has no Go test harness; document the manual DevTools steps
    (Application → Service Workers → Offline, and Cache Storage inspection).

---

## 6. Suggested sequence

| Phase | Scope | Effort | User-visible effect |
| --- | --- | --- | --- |
| 0 | substitutions single fetch, `Server-Timing`, self-host/drop CDN assets | ~1–2 h | substitutions ~250 ms faster; less first-paint jank |
| 1 | per-session SWR cache, write invalidation, warm-at-login, tests | ~½ day | repeat clicks < 20 ms server-side; cold dashboard warm |
| 2 | SW page cache, prefetch, logout wipe, view transitions, offline copy | ~½ day | clicks paint instantly from device; works offline |

Risks and mitigations are folded into each section: short fresh windows, hard
max-age, write-through invalidation, wipe on logout/expiry, no error caching,
and a per-session entry cap.
