package api

import (
	"encoding/base64"
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

func basicAuth(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
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

	var resp TokenInfo
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ID == "" || resp.Secret == "" {
		t.Fatal("expected non-empty ID and Secret")
	}
}

func TestScaleAPIRequiresAuth(t *testing.T) {
	handler, _ := setupTestAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/scalemgmt/v2/filesystems", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestScaleAPIWithAuth(t *testing.T) {
	handler, store := setupTestAPI(t)

	token, _ := store.Create([]string{"gpfs0"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/scalemgmt/v2/filesystems", nil)
	req.Header.Set("Authorization", basicAuth("admin", token.Secret))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetCluster(t *testing.T) {
	handler, store := setupTestAPI(t)

	token, _ := store.Create([]string{"gpfs0"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/scalemgmt/v2/cluster", nil)
	req.Header.Set("Authorization", basicAuth("admin", token.Secret))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetFilesystemAccessDenied(t *testing.T) {
	handler, store := setupTestAPI(t)

	token, _ := store.Create([]string{"gpfs0"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/scalemgmt/v2/filesystems/gpfs1", nil)
	req.Header.Set("Authorization", basicAuth("admin", token.Secret))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Returns 200 with filesystem info for Scale API compatibility
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetFilesystemAllowed(t *testing.T) {
	handler, store := setupTestAPI(t)

	token, _ := store.Create([]string{"gpfs0"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/scalemgmt/v2/filesystems/gpfs0", nil)
	req.Header.Set("Authorization", basicAuth("admin", token.Secret))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestJobEndpoint(t *testing.T) {
	handler, store := setupTestAPI(t)

	token, _ := store.Create([]string{"gpfs0"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/scalemgmt/v2/jobs/12345", nil)
	req.Header.Set("Authorization", basicAuth("admin", token.Secret))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
