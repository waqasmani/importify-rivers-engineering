package httpapi_test

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/httpapi"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/testutil"
)

func newEnvNoSetup(t *testing.T, url string) *testutil.Client {
	d, err := store.Open(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	cfg := testutil.Config(t, url)
	api, err := httpapi.New(cfg, d, slog.New(slog.NewTextHandler(io.Discard, nil)), httpapi.Deps{})
	if err != nil {
		t.Fatal(err)
	}
	if err := api.PrepareSetup(t.Context()); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.Routes())
	t.Cleanup(srv.Close)
	return testutil.NewClient(t, srv)
}
