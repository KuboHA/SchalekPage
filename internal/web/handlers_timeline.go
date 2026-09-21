// Feature 4: notification history with paging. The dashboard's own
// notification panel is fed entirely from the cached login payload (see
// handleDashboard's use of edupage.Client.Notifications), which EduPage caps
// at roughly a month. This file adds the "load older" endpoint behind that
// panel's button (see templates/dashboard.html), backed by the real network
// request edupage.Client.NotificationsSince performs.
package web

import (
	"net/http"
	"time"
)

// defaultNotificationHistoryLookback is used when the request carries no (or
// an unparseable) "since" parameter: a month back, matching the coverage the
// login-payload-derived feed already provides, so a first, param-less hit of
// this endpoint is never a no-op.
const defaultNotificationHistoryLookback = -30 * 24 * time.Hour

// handleNotificationHistory serves GET /notifications/history?since=YYYY-MM-DD,
// the network-backed counterpart to the dashboard's login-payload-only
// notification feed. It renders just the "notificationCards" fragment
// (shared with dashboard.html) so the caller's JS can splice the result
// straight into the existing notifications panel; see the
// "load older" script in templates/dashboard.html.
func (s *Server) handleNotificationHistory(w http.ResponseWriter, r *http.Request, sess *Session) {
	now := s.now()
	since := parseSinceOrDefault(r.URL.Query().Get("since"), now)

	events, err := sess.Client.NotificationsSince(since)
	if err != nil {
		s.logger.Warn("notifications history: fetch failed", "since", since, "error", err)
	}

	notifications := buildNotificationViews(events, baseURL(sess.Subdomain), now)
	s.render(w, r, "notificationCards", notifications)
}

// parseSinceOrDefault parses raw as a "YYYY-MM-DD" date (the layout the
// dashboard's "load older" button sends), falling back to
// defaultNotificationHistoryLookback before now when raw is empty or fails
// to parse.
func parseSinceOrDefault(raw string, now time.Time) time.Time {
	if raw == "" {
		return now.Add(defaultNotificationHistoryLookback)
	}
	parsed, err := time.ParseInLocation(dateLayout, raw, now.Location())
	if err != nil {
		return now.Add(defaultNotificationHistoryLookback)
	}
	return parsed
}
