package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()

	handler, err := Handler()
	if err != nil {
		t.Fatalf("Handler() error = %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))

	return rec
}

func TestHandler_ServesTheAssets(t *testing.T) {
	for _, asset := range []string{"/app.css", "/app.js", "/api.js", "/ui.js", "/views.js", "/sw.js"} {
		t.Run(asset, func(t *testing.T) {
			rec := serve(t, asset)

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if rec.Body.Len() == 0 {
				t.Error("the asset is empty")
			}
		})
	}
}

// The interface routes itself, and a confirmation link lands on /verify, which
// is a screen rather than a file.
func TestHandler_ServesThePageForARouteWithNoFile(t *testing.T) {
	for _, route := range []string{"/", "/verify", "/tickets", "/manage"} {
		t.Run(route, func(t *testing.T) {
			rec := serve(t, route)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if !strings.Contains(rec.Body.String(), "<title>SeatHold</title>") {
				t.Error("the answer is not the page")
			}
		})
	}
}

// A missing asset says so. Serving the page instead would turn a typo in a
// script tag into a script that silently runs HTML.
func TestHandler_AMissingAssetIsNotThePage(t *testing.T) {
	rec := serve(t, "/nope.js")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// A path that climbs out of the embedded filesystem must not reach anything.
func TestHandler_RefusesToClimbOut(t *testing.T) {
	rec := serve(t, "/../../go.mod")

	if strings.Contains(rec.Body.String(), "module ticket-reservation") {
		t.Fatal("a file from outside the embedded filesystem was served")
	}
}
