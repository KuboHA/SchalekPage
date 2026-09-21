package edupage

import "errors"

// Sentinel errors returned by this package. Use errors.Is to test for them.
var (
	// ErrBadCredentials is returned when EduPage rejects the given username/password.
	ErrBadCredentials = errors.New("edupage: bad credentials")

	// ErrCaptcha is returned when EduPage requires solving a captcha to log in
	// (typically after too many failed attempts).
	ErrCaptcha = errors.New("edupage: captcha required")

	// ErrSecondFactorFailed is returned when completing two-factor authentication
	// fails (wrong/expired code, expired session, or too much delay).
	ErrSecondFactorFailed = errors.New("edupage: second factor failed")

	// ErrMissingData is returned when EduPage's response is missing data this
	// package expected to find.
	ErrMissingData = errors.New("edupage: missing data")

	// ErrNotLoggedIn is returned when a method that requires an authenticated
	// session is called before Login/Restore has populated session data.
	ErrNotLoggedIn = errors.New("edupage: not logged in")

	// ErrSessionExpired is returned when an endpoint that depends on
	// Client.GsecHash() reports it stale (EduPage signals this with a
	// "reload" key in place of real data) and a single transparent
	// Restore() + retry did not resolve it either.
	ErrSessionExpired = errors.New("edupage: session expired")
)
