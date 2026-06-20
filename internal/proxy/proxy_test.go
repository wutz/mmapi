package proxy

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wutz/mmapi/internal/auth"
	"github.com/wutz/mmapi/internal/config"
)

// setupTestProxy creates a mock GUI backend and an mmapi proxy pointing to it
func setupTestProxy(t *testing.T) (*httptest.Server, http.Handler, *auth.TokenStore) {
	t.Helper()

	// Mock GPFS GUI backend
	guiRequests := make(chan *http.Request, 10)
	guiBackend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		guiRequests <- r
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":{"code":200,"message":""},"filesystems":[{"name":"fs0"}]}`))
	}))
	t.Cleanup(guiBackend.Close)

	cfg := &config.Config{
		DataDir:      t.TempDir(),
		TLS:          false,
		GuiURL:       guiBackend.URL,
		GuiUsername:  "admin",
		GuiPassword:  "Admin@123",
	}

	tokenStore := auth.NewTokenStore(cfg)
	proxy := New(cfg, tokenStore)

	return guiBackend, proxy, tokenStore
}

func makeRequest(method, path, token string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:"+token)))
	}
	return req
}

func TestProxyUnauthenticated(t *testing.T) {
	_, proxy, _ := setupTestProxy(t)

	req := makeRequest("GET", "/scalemgmt/v2/filesystems/fs0", "")
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestProxyInvalidToken(t *testing.T) {
	_, proxy, _ := setupTestProxy(t)

	req := makeRequest("GET", "/scalemgmt/v2/filesystems/fs0", "invalid_token")
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestProxyFilesystemAccessGranted(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"}, nil)

	req := makeRequest("GET", "/scalemgmt/v2/filesystems/fs0", token.Secret)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProxyFilesystemAccessDenied(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"}, nil)

	req := makeRequest("GET", "/scalemgmt/v2/filesystems/fs1", token.Secret)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestProxyFilesetAccessGranted(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"}, []string{"pvc-aaa"})

	req := makeRequest("GET", "/scalemgmt/v2/filesystems/fs0/filesets/pvc-aaa", token.Secret)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProxyFilesetAccessDenied(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"}, []string{"pvc-aaa"})

	req := makeRequest("GET", "/scalemgmt/v2/filesystems/fs0/filesets/pvc-bbb", token.Secret)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestProxyFilesetAccessAllWhenNoRestriction(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	// Token with allowedFs but no allowedFileset restriction
	token, _ := tokens.Create([]string{"fs0"}, nil)

	req := makeRequest("GET", "/scalemgmt/v2/filesystems/fs0/filesets/any-fileset", token.Secret)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProxyClusterEndpointNoFsCheck(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"}, nil)

	// /cluster endpoint has no filesystem in path, should pass
	req := makeRequest("GET", "/scalemgmt/v2/cluster", token.Secret)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProxyNonScalemgmtPath(t *testing.T) {
	_, proxy, _ := setupTestProxy(t)

	req := makeRequest("GET", "/api/v1/tokens", "")
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestProxyCreateFilesetBypassDenied(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	// Token restricted to a single fileset must NOT be able to create a
	// differently-named fileset via the create endpoint (body carries the name).
	token, _ := tokens.Create([]string{"fs0"}, []string{"pvc-aaa"})

	req := httptest.NewRequest("POST", "/scalemgmt/v2/filesystems/fs0/filesets",
		strings.NewReader(`{"filesetName":"pvc-evil","inodeSpace":"new"}`))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:"+token.Secret)))
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 (bypass blocked), got %d: %s", w.Code, w.Body.String())
	}
}

func TestProxyCreateFilesetAllowedFileset(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"}, []string{"pvc-aaa"})

	req := httptest.NewRequest("POST", "/scalemgmt/v2/filesystems/fs0/filesets",
		strings.NewReader(`{"filesetName":"pvc-aaa","inodeSpace":"new"}`))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:"+token.Secret)))
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProxySetQuotaBypassDenied(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	// quota set names the target fileset in the body (objectName), so a token
	// restricted to pvc-aaa must not set quota on pvc-evil.
	token, _ := tokens.Create([]string{"fs0"}, []string{"pvc-aaa"})

	req := httptest.NewRequest("POST", "/scalemgmt/v2/filesystems/fs0/quotas",
		strings.NewReader(`{"operationType":"setQuota","quotaType":"fileset","objectName":"pvc-evil","blockSoftLimit":"1","blockHardLimit":"2"}`))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:"+token.Secret)))
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 (quota bypass blocked), got %d: %s", w.Code, w.Body.String())
	}
}

func TestExtractFsAndFileset(t *testing.T) {
	tests := []struct {
		path         string
		expectedFs   string
		expectedFset string
	}{
		{"/scalemgmt/v2/filesystems/fs0", "fs0", ""},
		{"/scalemgmt/v2/filesystems/fs0/filesets", "fs0", ""},
		{"/scalemgmt/v2/filesystems/fs0/filesets/pvc-xxx", "fs0", "pvc-xxx"},
		{"/scalemgmt/v2/filesystems/fs0/filesets/pvc-xxx/link", "fs0", "pvc-xxx"},
		{"/scalemgmt/v2/filesystems/fs0/quotas", "fs0", ""},
		{"/scalemgmt/v2/cluster", "", ""},
		{"/scalemgmt/v2/nodes/node1/health/states", "", ""},
	}

	for _, tt := range tests {
		fs, fset := extractFsAndFileset(tt.path)
		if fs != tt.expectedFs || fset != tt.expectedFset {
			t.Errorf("extractFsAndFileset(%q) = (%q, %q), want (%q, %q)",
				tt.path, fs, fset, tt.expectedFs, tt.expectedFset)
		}
	}
}
