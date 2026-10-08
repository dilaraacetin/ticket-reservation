package handler

import (
	"net/http"
	"strings"
)

// contentSecurityPolicy is what the browser is allowed to load.
//
// 'self' throughout and no 'unsafe-inline' anywhere: the bundled page keeps its
// script and its stylesheet in their own files, so a script injected into the
// markup has nothing to run under. frame-ancestors 'none' is the modern
// X-Frame-Options and covers nested frames, which that header does not.
var contentSecurityPolicy = strings.Join([]string{
	"default-src 'self'",
	"script-src 'self'",
	"style-src 'self'",
	// Posters are hosted wherever the venue keeps them, so this one directive
	// has to admit remote origins. The cost is real and bounded: a poster URL is
	// set by an administrator, and a remote image lets that host see the IP of
	// everyone who browses the catalogue. https only, so a poster cannot be the
	// thing that makes the page mixed content.
	"img-src 'self' data: https:",
	"connect-src 'self'",
	"form-action 'self'",
	"frame-ancestors 'none'",
	"base-uri 'none'",
	"object-src 'none'",
}, "; ")

// SecurityHeaders sets the answers a browser needs before it will refuse to do
// something unsafe on the caller's behalf.
//
// There is no Strict-Transport-Security here. It would be a promise this
// deployment cannot keep: nothing terminates TLS yet, and a browser that has
// been told to use HTTPS for a year will not fall back when there is none.
// Whatever eventually terminates TLS is where that header belongs.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()

		// The strict policy by default. A handler that genuinely needs a looser
		// one replaces this with Set before it writes anything, which is what the
		// API documentation page does; this runs first, so it cannot be the one
		// deciding who is exempt.
		header.Set("Content-Security-Policy", contentSecurityPolicy)

		// Stops a browser guessing that a JSON answer is really HTML, which is
		// how a reflected value becomes a script.
		header.Set("X-Content-Type-Options", "nosniff")

		// Covered by frame-ancestors above, kept for browsers that predate it.
		header.Set("X-Frame-Options", "DENY")

		// Paths here carry hold and event ids. No referrer means they are not
		// handed to whatever a user clicks through to.
		header.Set("Referrer-Policy", "no-referrer")

		next.ServeHTTP(w, r)
	})
}
