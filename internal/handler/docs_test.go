package handler

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"ticket-reservation/internal/domain"
)

func TestHandler_Docs(t *testing.T) {
	rec := do(&fakeService{}, http.MethodGet, "/docs", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", got)
	}
	if !strings.Contains(rec.Body.String(), specPath) {
		t.Errorf("the docs page does not point at %s", specPath)
	}
}

func TestHandler_OpenAPISpec(t *testing.T) {
	rec := do(&fakeService{}, http.MethodGet, specPath, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/yaml" {
		t.Errorf("Content-Type = %q, want application/yaml", got)
	}
}

// The tests below are what stops a hand written spec from quietly drifting away
// from the code. They read the document as YAML rather than searching it as text,
// because a substring can just as easily be found in a description.

func loadSpec(t *testing.T) map[string]any {
	t.Helper()

	var spec map[string]any
	if err := yaml.Unmarshal(openAPISpec, &spec); err != nil {
		t.Fatalf("the embedded spec is not valid YAML: %v", err)
	}

	return spec
}

// dig walks into nested objects and fails the test with the path that was missing.
func dig(t *testing.T, node any, path ...string) any {
	t.Helper()

	for i, key := range path {
		object, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("%s is not an object", strings.Join(path[:i], "."))
		}

		node, ok = object[key]
		if !ok {
			t.Fatalf("the spec has no %s", strings.Join(path[:i+1], "."))
		}
	}

	return node
}

func stringsAt(t *testing.T, node any) []string {
	t.Helper()

	items, ok := node.([]any)
	if !ok {
		t.Fatalf("expected a list, got %T", node)
	}

	values := make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			t.Fatalf("expected a string in the list, got %T", item)
		}

		values = append(values, value)
	}

	return values
}

// TestOpenAPISpec_SeatStatusEnumMatchesTheDomain pins the one list that exists
// twice: SeatStatus.String in the domain and the enum here.
func TestOpenAPISpec_SeatStatusEnumMatchesTheDomain(t *testing.T) {
	got := stringsAt(t, dig(t, loadSpec(t), "components", "schemas", "SeatStatus", "enum"))

	var want []string
	for status := domain.StatusAvailable; status <= domain.StatusReserved; status++ {
		want = append(want, status.String())
	}

	if !slices.Equal(got, want) {
		t.Errorf("documented statuses = %v, want %v", got, want)
	}
}

// TestOpenAPISpec_DocumentsEveryErrorCode walks the handler's own mapping table,
// so adding an error without documenting it fails here.
func TestOpenAPISpec_DocumentsEveryErrorCode(t *testing.T) {
	documented := stringsAt(t, dig(t, loadSpec(t),
		"components", "schemas", "Error", "properties", "error", "properties", "code", "enum"))

	for _, mapping := range errorMapping {
		if !slices.Contains(documented, mapping.code) {
			t.Errorf("the spec does not document the error code %q", mapping.code)
		}
	}

	// The fallback code never appears in the table, so it is checked separately.
	if !slices.Contains(documented, "internal_error") {
		t.Error("the spec does not document the internal_error code")
	}
}

func TestOpenAPISpec_DocumentsEveryRoute(t *testing.T) {
	paths, ok := dig(t, loadSpec(t), "paths").(map[string]any)
	if !ok {
		t.Fatal("paths is not an object")
	}

	// Kept in step with Routes by hand; the docs endpoints themselves are
	// deliberately left out of the spec, being documentation rather than API.
	routes := map[string][]string{
		"/health":                               {"get"},
		"/auth/register":                        {"post"},
		"/auth/login":                           {"post"},
		"/events":                               {"get"},
		"/events/{eventID}/seats":               {"get"},
		"/events/{eventID}/stream":              {"get"},
		"/events/{eventID}/seats/{seatID}/hold": {"post"},
		"/holds/{holdID}/confirm":               {"post"},
		"/holds/{holdID}":                       {"delete"},
	}

	for path, methods := range routes {
		item, ok := paths[path].(map[string]any)
		if !ok {
			t.Errorf("the spec does not document %s", path)

			continue
		}

		for _, method := range methods {
			if _, ok := item[method]; !ok {
				t.Errorf("the spec does not document %s %s", strings.ToUpper(method), path)
			}
		}
	}
}

// TestOpenAPISpec_EveryRefResolves catches the failure a YAML parser cannot: a
// $ref that points at nothing renders as an empty schema in Swagger UI instead of
// raising an error.
func TestOpenAPISpec_EveryRefResolves(t *testing.T) {
	spec := loadSpec(t)

	refs := collectRefs(spec)
	if len(refs) == 0 {
		t.Fatal("no $ref found, which means this test is not checking anything")
	}

	for _, ref := range refs {
		if !resolves(spec, ref) {
			t.Errorf("$ref %q does not resolve", ref)
		}
	}
}

func collectRefs(node any) []string {
	switch typed := node.(type) {
	case map[string]any:
		var refs []string

		for key, value := range typed {
			if key == "$ref" {
				if ref, ok := value.(string); ok {
					refs = append(refs, ref)
				}

				continue
			}

			refs = append(refs, collectRefs(value)...)
		}

		return refs
	case []any:
		var refs []string
		for _, value := range typed {
			refs = append(refs, collectRefs(value)...)
		}

		return refs
	default:
		return nil
	}
}

func resolves(spec map[string]any, ref string) bool {
	trimmed, ok := strings.CutPrefix(ref, "#/")
	if !ok {
		return false
	}

	var node any = spec
	for _, part := range strings.Split(trimmed, "/") {
		object, ok := node.(map[string]any)
		if !ok {
			return false
		}

		node, ok = object[part]
		if !ok {
			return false
		}
	}

	return true
}
