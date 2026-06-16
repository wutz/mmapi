package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/wutz/mmapi/internal/auth"
	"github.com/wutz/mmapi/internal/config"
)

func setupTestAPI(t *testing.T) (http.Handler, *auth.TokenStore) {
	t.Helper()
	cfg := &config.Config{
		Mode:    config.ModeMultiFileset,
		DataDir: t.TempDir(),
		Device:  "gpfs0",
		Port:    8080,
	}

	router := chi.NewMux()
	humaAPI := humachi.New(router, huma.DefaultConfig("mmapi-test", "1.0.0"))
	tokenStore := auth.NewTokenStore(cfg)
	authMw := auth.Middleware(tokenStore)

	RegisterRoutes(humaAPI, cfg, tokenStore, authMw)

	return router, tokenStore
}

func TestCreateToken(t *testing.T) {
	handler, _ := setupTestAPI(t)

	body := `{"allowedFs":["gpfs0"],"allowedFileset":["fs-a"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		ID     string   `json:"id"`
		Secret string   `json:"secret"`
		AllowedFS []string `json:"allowedFs"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.ID == "" || resp.Secret == "" {
		t.Fatal("expected non-empty ID and Secret")
	}
}

func TestListTokens(t *testing.T) {
	handler, _ := setupTestAPI(t)

	// Create a token first
	body := `{"allowedFs":["gpfs0"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// List tokens
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tokens", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestVolumeEndpointRequiresAuth(t *testing.T) {
	handler, _ := setupTestAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/volumes", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestVolumeEndpointWithAuth(t *testing.T) {
	handler, store := setupTestAPI(t)

	token, _ := store.Create([]string{"gpfs0"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/volumes", nil)
	req.Header.Set("Authorization", "Bearer "+token.Secret)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Will fail because gpfs commands aren't available in test, but auth should pass
	// We check it's not 401
	if w.Code == http.StatusUnauthorized {
		t.Fatal("expected auth to pass")
	}
}

func TestDeleteToken(t *testing.T) {
	handler, _ := setupTestAPI(t)

	// Create
	body := `{"allowedFs":["gpfs0"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var resp struct{ ID string `json:"id"` }
	json.Unmarshal(w.Body.Bytes(), &resp)

	// Delete
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/tokens/"+resp.ID, nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent && w.Code != http.StatusOK {
		t.Fatalf("expected 200/204, got %d: %s", w.Code, w.Body.String())
	}
}
