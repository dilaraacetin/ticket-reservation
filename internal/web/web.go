package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed static
var files embed.FS

// Handler serves the interface.
//
// A path with no file behind it gets index.html rather than a 404, because the
// interface routes itself: a confirmation link lands on /verify, which is a
// screen rather than a file. Only paths that look like pages are treated this
// way — anything with an extension is a missing asset and says so, which is the
// difference between a typo in a script tag and a page nobody wrote.
func Handler() (http.Handler, error) {
	root, err := fs.Sub(files, "static")
	if err != nil {
		return nil, err
	}

	page, err := fs.ReadFile(root, "index.html")
	if err != nil {
		return nil, err
	}

	assets := http.FileServerFS(root)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")

		if name == "" || name == "index.html" {
			writePage(w, page)

			return
		}

		if _, err := fs.Stat(root, name); err == nil {
			assets.ServeHTTP(w, r)

			return
		}

		if path.Ext(name) != "" {
			http.NotFound(w, r)

			return
		}

		writePage(w, page)
	}), nil
}

func writePage(w http.ResponseWriter, page []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Never cached: the page is the one thing that has to be current, because
	// everything else it loads is named from inside it.
	w.Header().Set("Cache-Control", "no-store")

	_, _ = w.Write(page)
}
