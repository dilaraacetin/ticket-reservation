package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var files embed.FS

// Handler serves the interface. Unknown paths fall through to a 404 from the
// file server itself, so the API's own 404s are unaffected.
func Handler() (http.Handler, error) {
	root, err := fs.Sub(files, "static")
	if err != nil {
		return nil, err
	}

	return http.FileServerFS(root), nil
}
