package handler

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hello"))
	})
}

func TestRequestID(t *testing.T) {
	t.Run("generates an id and echoes it back", func(t *testing.T) {
		var seen string

		h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen = RequestIDFromContext(r.Context())
		}))

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

		if seen == "" {
			t.Error("no request id reached the handler")
		}
		if got := rec.Header().Get(requestIDHeader); got != seen {
			t.Errorf("header = %q, want %q", got, seen)
		}
	})

	t.Run("keeps an id the client supplied", func(t *testing.T) {
		var seen string

		h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen = RequestIDFromContext(r.Context())
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(requestIDHeader, "from-the-client")

		h.ServeHTTP(httptest.NewRecorder(), req)

		if seen != "from-the-client" {
			t.Errorf("request id = %q, want from-the-client", seen)
		}
	})

	t.Run("an empty context has no id", func(t *testing.T) {
		if got := RequestIDFromContext(httptest.NewRequest(http.MethodGet, "/", nil).Context()); got != "" {
			t.Errorf("request id = %q, want empty", got)
		}
	})
}

func TestLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	h := Chain(okHandler(), RequestID, Logging(logger))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/somewhere", nil))

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log line %q is not JSON: %v", buf.String(), err)
	}

	if line["status"] != float64(http.StatusTeapot) {
		t.Errorf("status = %v, want %d", line["status"], http.StatusTeapot)
	}
	if line["path"] != "/somewhere" {
		t.Errorf("path = %v, want /somewhere", line["path"])
	}
	if line["bytes"] != float64(len("hello")) {
		t.Errorf("bytes = %v, want %d", line["bytes"], len("hello"))
	}
	if id, _ := line["requestId"].(string); id == "" {
		t.Error("requestId is missing from the log line")
	}
}

func TestLogging_ImpliedStatus(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	bodyOnly := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("no explicit status"))
	})

	Logging(logger)(bodyOnly).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log line %q is not JSON: %v", buf.String(), err)
	}
	if line["status"] != float64(http.StatusOK) {
		t.Errorf("status = %v, want %d", line["status"], http.StatusOK)
	}
}

func TestRecovery(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("something went very wrong")
	})

	rec := httptest.NewRecorder()

	Recovery(logger)(panicking).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
	}
	if body.Error.Code != "internal_error" {
		t.Errorf("code = %q, want internal_error", body.Error.Code)
	}
	if strings.Contains(rec.Body.String(), "something went very wrong") {
		t.Errorf("body leaks the panic value: %s", rec.Body.String())
	}
	if !strings.Contains(buf.String(), "something went very wrong") {
		t.Errorf("the panic value was not logged: %s", buf.String())
	}
}

func TestChain_OrderIsOutermostFirst(t *testing.T) {
	var order []string

	record := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}

	h := Chain(okHandler(), record("first"), record("second"), record("third"))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	want := []string{"first", "second", "third"}
	if len(order) != len(want) {
		t.Fatalf("ran %v, want %v", order, want)
	}

	for i, name := range want {
		if order[i] != name {
			t.Errorf("position %d = %q, want %q", i, order[i], name)
		}
	}
}
