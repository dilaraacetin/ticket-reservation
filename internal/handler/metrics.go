package handler

import (
	"context"
	"net/http"
	"time"
)

// Observer is the slice of the metrics this package needs, declared here on the
// consuming side like every other dependency.
type Observer interface {
	RecordRequest(method, route string, status int, took time.Duration)
	RecordFailure(code string)
	RequestStarted()
	RequestFinished()
}

// RouteResolver names the route a request would match, without serving it.
// http.ServeMux is one.
type RouteResolver interface {
	Handler(r *http.Request) (http.Handler, string)
}

// requestNotes is what the inside of the chain tells the metrics middleware on
// the outside of it.
//
// A pointer in the context, rather than plain context values, because each
// middleware replaces the request with a copy of itself: a value written
// further in would land on a copy the outer middleware never sees. Copies do
// carry the pointer, so writing through it reaches everyone.
type requestNotes struct {
	code string
}

type notesKey struct{}

func notesFrom(ctx context.Context) *requestNotes {
	notes, _ := ctx.Value(notesKey{}).(*requestNotes)

	return notes
}

// noteFailure records the code the caller was given. Called from the single
// place that writes an error answer, so a refusal is counted wherever in the
// chain it came from.
func noteFailure(ctx context.Context, code string) {
	if notes := notesFrom(ctx); notes != nil {
		notes.code = code
	}
}

// Measure counts and times every request.
//
// It is the outermost middleware, so what it times is what the caller waited
// for, including the queueing every other middleware does.
func Measure(observer Observer, routes RouteResolver) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			observer.RequestStarted()
			defer observer.RequestFinished()

			notes := &requestNotes{}
			r = r.WithContext(context.WithValue(r.Context(), notesKey{}, notes))

			// Resolved here rather than read back from the mux, because a request
			// the rate limiter or the idempotency check turns away never reaches
			// the mux at all, and labelling every refusal "unmatched" would hide
			// exactly the route that is being hammered.
			//
			// The pattern, never r.URL.Path: one time series per hold id is how a
			// metrics store is brought down by its own data.
			route := "unmatched"
			if _, pattern := routes.Handler(r); pattern != "" {
				route = pattern
			}

			started := time.Now()
			recorder := &statusRecorder{ResponseWriter: w}

			next.ServeHTTP(recorder, r)

			observer.RecordRequest(r.Method, route, recorder.status, time.Since(started))

			if notes.code != "" {
				observer.RecordFailure(notes.code)
			}
		})
	}
}
