package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The files are embedded, and an embed that matches nothing fails at run time
// rather than at build time. This is the same trap the migrations fell into.
func TestHandler_ServesTheEmbeddedFiles(t *testing.T) {
	handler, err := Handler()
	if err != nil {
		t.Fatalf("Handler() error = %v", err)
	}

	tests := []struct {
		name        string
		path        string
		wantStatus  int
		wantContent string
		wantInBody  string
	}{
		{"the page itself", "/", http.StatusOK, "text/html", "<title>Ticket Reservation</title>"},
		// The file server sends /index.html to the canonical /, which is its own
		// behaviour rather than something to work around.
		{"index by name redirects to the root", "/index.html", http.StatusMovedPermanently, "", ""},
		{"styles", "/app.css", http.StatusOK, "text/css", ".seat"},
		{"behaviour", "/app.js", http.StatusOK, "javascript", "Idempotency-Key"},
		{"anything else", "/nope", http.StatusNotFound, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantContent != "" && !strings.Contains(rec.Header().Get("Content-Type"), tt.wantContent) {
				t.Errorf("Content-Type = %q, want it to mention %q", rec.Header().Get("Content-Type"), tt.wantContent)
			}
			if tt.wantInBody != "" && !strings.Contains(rec.Body.String(), tt.wantInBody) {
				t.Errorf("the body does not contain %q", tt.wantInBody)
			}
		})
	}
}

// A file added to the folder but not to the page is dead weight, and a file the
// page asks for but that is missing is a broken interface. This checks the set.
func TestEmbeddedFilesAreTheOnesThePageUses(t *testing.T) {
	root, err := fs.Sub(files, "static")
	if err != nil {
		t.Fatalf("fs.Sub() error = %v", err)
	}

	entries, err := fs.ReadDir(root, ".")
	if err != nil {
		t.Fatalf("reading the embedded folder failed: %v", err)
	}

	found := make(map[string]bool, len(entries))
	for _, entry := range entries {
		found[entry.Name()] = true
	}

	for _, want := range []string{"index.html", "app.css", "app.js"} {
		if !found[want] {
			t.Errorf("%s is not embedded", want)
		}
	}

	page, err := fs.ReadFile(root, "index.html")
	if err != nil {
		t.Fatalf("reading index.html failed: %v", err)
	}

	for _, asset := range []string{"/app.css", "/app.js"} {
		if !strings.Contains(string(page), asset) {
			t.Errorf("the page does not reference %s", asset)
		}
	}
}
