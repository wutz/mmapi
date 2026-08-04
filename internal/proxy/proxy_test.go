package proxy

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/wutz/mmapi/internal/auth"
	"github.com/wutz/mmapi/internal/config"
)

// setupTestProxy creates a mock GUI backend and an mmapi proxy pointing to it
func setupTestProxy(t *testing.T) (*httptest.Server, http.Handler, *auth.TokenStore) {
	t.Helper()

	// Mock GPFS GUI backend. It echoes the request body back so tests can
	// assert that the proxy forwards write payloads untouched.
	guiBackend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":{"code":200,"message":""},"received":` +
			strconv.Quote(string(body)) + `}`))
	}))
	t.Cleanup(guiBackend.Close)

	cfg := &config.Config{
		DataDir:     t.TempDir(),
		TLS:         false,
		GuiURL:      guiBackend.URL,
		GuiUsername: "admin",
		GuiPassword: "Admin@123",
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

	token, _ := tokens.Create([]string{"fs0"})

	req := makeRequest("GET", "/scalemgmt/v2/filesystems/fs0", token.Secret)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProxyFilesystemAccessDenied(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"})

	req := makeRequest("GET", "/scalemgmt/v2/filesystems/fs1", token.Secret)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestProxyFilesetPathAccessGranted(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"})

	req := makeRequest("GET", "/scalemgmt/v2/filesystems/fs0/filesets/pvc-aaa", token.Secret)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProxyFilesetPathAccessDeniedOnForeignFS(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"})

	req := makeRequest("GET", "/scalemgmt/v2/filesystems/fs1/filesets/pvc-aaa", token.Secret)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

// A token owning a filesystem owns every fileset in it, including ones the
// CSI driver provisions dynamically under names nobody could allowlist ahead
// of time.
func TestProxyAnyFilesetInAllowedFS(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"})

	for _, name := range []string{"pvc-aaa", "pvc-e2a1f0c4-dead-beef", "any-fileset"} {
		req := makeRequest("GET", "/scalemgmt/v2/filesystems/fs0/filesets/"+name, token.Secret)
		w := httptest.NewRecorder()
		proxy.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("fileset %q: expected 200, got %d: %s", name, w.Code, w.Body.String())
		}
	}
}

func TestProxyClusterEndpointNoFsCheck(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"})

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

func TestProxyCreateFilesetAnyName(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"})

	req := httptest.NewRequest("POST", "/scalemgmt/v2/filesystems/fs0/filesets",
		strings.NewReader(`{"filesetName":"pvc-anything","inodeSpace":"new"}`))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:"+token.Secret)))
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// The proxy must forward the payload untouched now that it no longer
	// reads and rebuilds the body.
	if !strings.Contains(w.Body.String(), `pvc-anything`) {
		t.Fatalf("expected body to reach the GUI intact, got %s", w.Body.String())
	}
}

func TestProxyCreateFilesetDeniedOnForeignFS(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"})

	req := httptest.NewRequest("POST", "/scalemgmt/v2/filesystems/fs1/filesets",
		strings.NewReader(`{"filesetName":"pvc-evil","inodeSpace":"new"}`))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:"+token.Secret)))
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProxySetQuotaDeniedOnForeignFS(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"})

	req := httptest.NewRequest("POST", "/scalemgmt/v2/filesystems/fs1/quotas",
		strings.NewReader(`{"operationType":"setQuota","quotaType":"fileset","objectName":"pvc-evil","blockSoftLimit":"1","blockHardLimit":"2"}`))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:"+token.Secret)))
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProxySetQuotaAllowedFS(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"})

	req := httptest.NewRequest("POST", "/scalemgmt/v2/filesystems/fs0/quotas",
		strings.NewReader(`{"operationType":"setQuota","quotaType":"fileset","objectName":"pvc-aaa","blockSoftLimit":"1","blockHardLimit":"2"}`))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:"+token.Secret)))
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// User and group quotas live under the fileset-scoped quota collection when a
// filesystem has per-fileset quota enabled. Those paths must stay behind the
// same filesystem check as everything else.
func TestProxyFilesetQuotaPathAccess(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"})

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		expected int
	}{
		{"list allowed", "GET", "/scalemgmt/v2/filesystems/fs0/filesets/fset1/quotas", "", http.StatusOK},
		{"list filtered allowed", "GET", "/scalemgmt/v2/filesystems/fs0/filesets/fset1/quotas?filter=quotaType=USR", "", http.StatusOK},
		{"list denied", "GET", "/scalemgmt/v2/filesystems/fs1/filesets/fset1/quotas", "", http.StatusForbidden},
		{"set user allowed", "POST", "/scalemgmt/v2/filesystems/fs0/filesets/fset1/quotas",
			`{"operationType":"setQuota","quotaType":"USR","objectName":"ubuntu","blockSoftLimit":"1G","blockHardLimit":"2G"}`, http.StatusOK},
		{"set user denied", "POST", "/scalemgmt/v2/filesystems/fs1/filesets/fset1/quotas",
			`{"operationType":"setQuota","quotaType":"USR","objectName":"ubuntu","blockSoftLimit":"1G","blockHardLimit":"2G"}`, http.StatusForbidden},
		{"set group allowed", "POST", "/scalemgmt/v2/filesystems/fs0/filesets/fset1/quotas",
			`{"operationType":"setQuota","quotaType":"GRP","objectName":"ubuntu","blockSoftLimit":"1G","blockHardLimit":"2G"}`, http.StatusOK},
		{"set group denied", "POST", "/scalemgmt/v2/filesystems/fs1/filesets/fset1/quotas",
			`{"operationType":"setQuota","quotaType":"GRP","objectName":"ubuntu","blockSoftLimit":"1G","blockHardLimit":"2G"}`, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body io.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.path, body)
			req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:"+token.Secret)))
			w := httptest.NewRecorder()
			proxy.ServeHTTP(w, req)

			if w.Code != tt.expected {
				t.Fatalf("expected %d, got %d: %s", tt.expected, w.Code, w.Body.String())
			}
		})
	}
}

// The quota payload carries the user or group name, which the proxy must not
// touch — it authorizes on the filesystem in the path alone.
func TestProxyUserQuotaBodyForwardedIntact(t *testing.T) {
	_, proxy, tokens := setupTestProxy(t)

	token, _ := tokens.Create([]string{"fs0"})

	payload := `{"operationType":"setQuota","quotaType":"USR","objectName":"ubuntu","blockSoftLimit":"1G","blockHardLimit":"2G","filesSoftLimit":"1000","filesHardLimit":"2000"}`
	req := httptest.NewRequest("POST", "/scalemgmt/v2/filesystems/fs0/filesets/fset1/quotas",
		strings.NewReader(payload))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:"+token.Secret)))
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	for _, want := range []string{`quotaType`, `USR`, `ubuntu`, `filesHardLimit`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("expected %q to reach the GUI, got %s", want, w.Body.String())
		}
	}
}

func TestExtractFS(t *testing.T) {
	tests := []struct {
		path       string
		expectedFs string
	}{
		{"/scalemgmt/v2/filesystems/fs0", "fs0"},
		{"/scalemgmt/v2/filesystems/fs0/filesets", "fs0"},
		{"/scalemgmt/v2/filesystems/fs0/filesets/pvc-xxx", "fs0"},
		{"/scalemgmt/v2/filesystems/fs0/filesets/pvc-xxx/link", "fs0"},
		{"/scalemgmt/v2/filesystems/fs0/quotas", "fs0"},
		{"/scalemgmt/v2/filesystems/fs0/filesets/fset1/quotas", "fs0"},
		{"/scalemgmt/v2/filesystems/fs0/quotadefaults", "fs0"},
		{"/scalemgmt/v2/cluster", ""},
		{"/scalemgmt/v2/nodes/node1/health/states", ""},
	}

	for _, tt := range tests {
		if fs := extractFS(tt.path); fs != tt.expectedFs {
			t.Errorf("extractFS(%q) = %q, want %q", tt.path, fs, tt.expectedFs)
		}
	}
}
