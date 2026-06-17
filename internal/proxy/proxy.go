package proxy

import (
	"crypto/tls"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/wutz/mmapi/internal/auth"
	"github.com/wutz/mmapi/internal/config"
)

// New creates a reverse proxy that forwards /scalemgmt/ requests to the
// real GPFS GUI, replacing auth with the GUI admin credentials.
func New(cfg *config.Config, tokens *auth.TokenStore) http.Handler {
	guiURL, err := url.Parse(cfg.GuiURL)
	if err != nil {
		slog.Error("invalid gui_url", "url", cfg.GuiURL, "error", err)
		panic("invalid gui_url: " + err.Error())
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = guiURL.Scheme
			req.URL.Host = guiURL.Host
			req.Host = guiURL.Host
			// Replace token auth with GUI admin basic auth
			req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString(
				[]byte(cfg.GuiUsername+":"+cfg.GuiPassword)))
		},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		ModifyResponse: func(resp *http.Response) error {
			// Log response status for debugging
			slog.Debug("proxy response", "status", resp.StatusCode, "url", resp.Request.URL.Path)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("proxy error", "url", r.URL.Path, "error", err)
			http.Error(w, `{"status":{"code":502,"message":"upstream error"}}`, http.StatusBadGateway)
		},
	}

	return &handler{
		proxy:  proxy,
		tokens: tokens,
		cfg:    cfg,
	}
}

type handler struct {
	proxy  *httputil.ReverseProxy
	tokens *auth.TokenStore
	cfg    *config.Config
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Token auth check for /scalemgmt/ paths
	if strings.HasPrefix(r.URL.Path, "/scalemgmt/") {
		token := h.authenticate(r)
		if token == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"status":{"code":401,"message":"unauthorized"}}`))
			return
		}

		// Access control: extract filesystem from URL and check permission
		fs := extractFilesystem(r.URL.Path)
		if fs != "" {
			if err := h.tokens.CheckAccess(token, fs, ""); err != nil {
				slog.Warn("access denied", "fs", fs, "token", token.ID, "error", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"status":{"code":403,"message":"access denied"}}`))
				return
			}
		}

		h.proxy.ServeHTTP(w, r)
		return
	}

	// Non-scalemgmt paths: return 404
	http.NotFound(w, r)
}

func (h *handler) authenticate(r *http.Request) *auth.Token {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil
	}

	if strings.HasPrefix(authHeader, "Basic ") {
		decoded, err := base64.StdEncoding.DecodeString(authHeader[6:])
		if err != nil {
			return nil
		}
		_, pass, ok := strings.Cut(string(decoded), ":")
		if !ok {
			return nil
		}
		token, valid := h.tokens.Validate(pass)
		if !valid {
			return nil
		}
		return token
	}

	return nil
}

// extractFilesystem extracts the filesystem name from a scalemgmt URL path.
// e.g., /scalemgmt/v2/filesystems/fs0/filesets -> "fs0"
func extractFilesystem(path string) string {
	const prefix = "/scalemgmt/v2/filesystems/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	if idx := strings.Index(rest, "/"); idx != -1 {
		return rest[:idx]
	}
	return rest
}
