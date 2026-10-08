package handler

import (
	"crypto/rand"
	_ "embed"
	"net/http"
	"strings"
)

// swaggerCDN is where the documentation page gets its assets. It is the one
// third party this service loads anything from, and the reason /docs needs a
// policy of its own.
const swaggerCDN = "https://unpkg.com"

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
  <script nonce="{{nonce}}">
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

// docsPolicy is the documentation page's own Content-Security-Policy.
//
// It is looser than everywhere else because this page needs a CDN and one inline
// script. The inline block is allowed by a nonce rather than 'unsafe-inline',
// which would also allow anything injected into the markup. style-src keeps
// 'unsafe-inline' because Swagger UI writes styles at run time; that is the
// weakest line here, and vendoring the assets instead of loading them from a CDN
// is what would remove both it and the third party.
func docsPolicy(nonce string) string {
	return strings.Join([]string{
		"default-src 'none'",
		"script-src 'nonce-" + nonce + "' " + swaggerCDN,
		"style-src 'unsafe-inline' " + swaggerCDN,
		"img-src 'self' data:",
		"font-src " + swaggerCDN + " data:",
		"connect-src 'self'",
		"frame-ancestors 'none'",
		"base-uri 'none'",
	}, "; ")
}

// docs serves the Swagger UI page.
func (h *Handler) docs(w http.ResponseWriter, r *http.Request) {
	// A fresh value per response. A nonce that repeats is a nonce an attacker can
	// reuse, which is the same as not having one.
	nonce := rand.Text()

	w.Header().Set("Content-Security-Policy", docsPolicy(nonce))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	page := strings.ReplaceAll(docsPage, "{{nonce}}", nonce)

	if _, err := w.Write([]byte(page)); err != nil {
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
