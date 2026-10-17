package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/go-chi/chi/v5"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/httpapi"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/testutil"
)

var (
	specOnce   sync.Once
	specDoc    *openapi3.T
	specRouter routers.Router
	specErr    error
)

func loadSpec() (*openapi3.T, routers.Router, error) {
	specOnce.Do(func() {
		raw, err := json.Marshal(httpapi.BuildOpenAPI("test"))
		if err != nil {
			specErr = err
			return
		}
		loader := openapi3.NewLoader()
		specDoc, specErr = loader.LoadFromData(raw)
		if specErr != nil {
			return
		}
		if specErr = specDoc.Validate(context.Background()); specErr != nil {
			return
		}
		specDoc.Servers = openapi3.Servers{{URL: "http://127.0.0.1"}}
		specRouter, specErr = gorillamux.NewRouter(specDoc)
	})
	return specDoc, specRouter, specErr
}

// contractValidator checks JSON responses against the OpenAPI document. Every integration test
// that uses newEnv is therefore also a contract test.
func contractValidator(t testing.TB) func(*http.Request, *testutil.Response) {
	_, router, err := loadSpec()
	if err != nil {
		t.Fatalf("OpenAPI document is invalid: %v", err)
	}
	return func(req *http.Request, res *testutil.Response) {
		t.Helper()
		if !strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") || res.Status == http.StatusTooManyRequests {
			return
		}
		probe, _ := http.NewRequest(req.Method, "http://127.0.0.1"+req.URL.RequestURI(), nil)
		route, params, err := router.FindRoute(probe)
		if err != nil {
			t.Errorf("contract: %s %s is not documented: %v", req.Method, req.URL.Path, err)
			return
		}
		in := &openapi3filter.RequestValidationInput{Request: probe, PathParams: params, Route: route,
			Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc}}
		err = openapi3filter.ValidateResponse(context.Background(), &openapi3filter.ResponseValidationInput{
			RequestValidationInput: in, Status: res.Status, Header: res.Header, Body: io.NopCloser(bytes.NewReader(res.Body)),
			Options: &openapi3filter.Options{IncludeResponseStatus: true, MultiError: false}})
		if err != nil {
			msg := err.Error()
			if len(msg) > 1500 {
				msg = msg[:1500]
			}
			t.Errorf("contract: %s %s -> %d does not match the OpenAPI document: %s", req.Method, req.URL.Path, res.Status, msg)
		}
	}
}

func TestOpenAPIDocumentsEveryRoute(t *testing.T) {
	e := newEnv(t)
	documented := map[string]bool{}
	for _, o := range httpapi.Operations() {
		key := o.Method + " " + o.Path
		if documented[key] {
			t.Errorf("operation documented twice: %s", key)
		}
		documented[key] = true
	}
	routed := map[string]bool{}
	_ = chi.Walk(e.api.Routes().(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.ReplaceAll(strings.TrimSuffix(strings.ReplaceAll(route, "/*", ""), "/"), "//", "/")
		routed[method+" "+route] = true
		return nil
	})
	var missing, stale []string
	for k := range routed {
		if !documented[k] {
			missing = append(missing, k)
		}
	}
	for k := range documented {
		if !routed[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("routes missing from the OpenAPI document:\n%s", strings.Join(missing, "\n"))
	}
	if len(stale) > 0 {
		t.Errorf("documented operations without a route:\n%s", strings.Join(stale, "\n"))
	}
	if _, _, err := loadSpec(); err != nil {
		t.Fatalf("OpenAPI document does not validate: %v", err)
	}
	// The served document matches the generated one.
	served := e.client().Do("GET", "/api/v1/openapi.json", nil).Must(t, 200)
	if served.Str("openapi") != "3.0.3" {
		t.Fatal("served document wrong")
	}
}

// TestCommittedOpenAPIIsCurrent keeps packages/api-types/openapi.json in sync. Run with
// REA_UPDATE_OPENAPI=1 to regenerate it.
func TestCommittedOpenAPIIsCurrent(t *testing.T) {
	path := filepath.Join("..", "..", "..", "packages", "api-types", "openapi.json")
	want, _ := json.MarshalIndent(httpapi.BuildOpenAPI("1.0.0"), "", "  ")
	want = append(want, '\n')
	if os.Getenv("REA_UPDATE_OPENAPI") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("packages/api-types/openapi.json is out of date; run `make generate` (REA_UPDATE_OPENAPI=1 go test ./services/internal/httpapi -run TestCommittedOpenAPIIsCurrent)")
	}
}
