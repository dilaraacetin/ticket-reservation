package handler

import (
	_ "embed"
	"net/http"
)

// openAPISpec is the API contract, compiled into the binary. go:embed cannot
// reach outside the package directory, which is why the file lives next to the
// code that serves it rather than in a top level api/ folder.
//
//go:embed openapi.yaml
var openAPISpec []byte

// specPath is where the spec is served, and what the docs page fetches.
const specPath = "/openapi.yaml"

// docsPage is Swagger UI pointed at our own spec. The assets come from a CDN, so
// the page needs internet access; the spec itself is served locally.
const docsPage = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Ticket Reservation API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js" crossorigin></script>
  <script>
    window.onload = () => {
      SwaggerUIBundle({
        url: "` + specPath + `",
        dom_id: "#swagger-ui",
        // Keeps the X-User-ID typed into Authorize across reloads, which makes
        // trying the write endpoints far less tedious.
        persistAuthorization: true,
        tryItOutEnabled: true,
      });
    };
  </script>
</body>
</html>
`

// docs serves the Swagger UI page.
func (h *Handler) docs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if _, err := w.Write([]byte(docsPage)); err != nil {
		h.logger.ErrorContext(r.Context(), "writing docs page failed", "err", err)
	}
}

// openAPI serves the spec itself.
func (h *Handler) openAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")

	if _, err := w.Write(openAPISpec); err != nil {
		h.logger.ErrorContext(r.Context(), "writing openapi spec failed", "err", err)
	}
}
