package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/wutz/mmapi/internal/auth"
	"github.com/wutz/mmapi/internal/config"
	"github.com/wutz/mmapi/internal/proxy"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	tokenStore := auth.NewTokenStore(cfg)

	mux := http.NewServeMux()

	// Proxy all /scalemgmt/ requests to the real GPFS GUI
	scaleProxy := proxy.New(cfg, tokenStore)
	mux.Handle("/scalemgmt/", scaleProxy)

	// Admin auth middleware for token management
	adminAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if cfg.AdminToken != "" {
				authHeader := r.Header.Get("Authorization")
				if authHeader != "Bearer "+cfg.AdminToken {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte(`{"error":"admin authentication required"}`))
					return
				}
			}
			next(w, r)
		}
	}

	// Token management API (requires admin token)
	mux.HandleFunc("POST /api/v1/tokens", adminAuth(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AllowedFS      []string `json:"allowedFs"`
			AllowedFileset []string `json:"allowedFileset,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}
		token, err := tokenStore.Create(body.AllowedFS, body.AllowedFileset)
		if err != nil {
			http.Error(w, `{"error":"failed to create token"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(token)
	}))

	mux.HandleFunc("GET /api/v1/tokens", adminAuth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenStore.List())
	}))

	mux.HandleFunc("DELETE /api/v1/tokens/{id}", adminAuth(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := tokenStore.Delete(id); err != nil {
			http.Error(w, `{"error":"failed to delete token"}`, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	// Logging middleware
	handler := logMiddleware(mux)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: handler,
	}

	go func() {
		if cfg.TLS {
			certFile, keyFile, err := ensureTLSCerts(cfg)
			if err != nil {
				slog.Error("failed to setup TLS", "error", err)
				os.Exit(1)
			}
			slog.Info("starting HTTPS server", "port", cfg.Port, "gui", cfg.GuiURL)
			if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
				slog.Error("server error", "error", err)
				os.Exit(1)
			}
		} else {
			slog.Info("starting HTTP server", "port", cfg.Port, "gui", cfg.GuiURL)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("server error", "error", err)
				os.Exit(1)
			}
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	slog.Info("server stopped")
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start).String(),
			"from", r.RemoteAddr,
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func ensureTLSCerts(cfg *config.Config) (string, string, error) {
	certFile := cfg.CertFile
	keyFile := cfg.KeyFile
	if certFile == "" {
		certFile = filepath.Join(cfg.DataDir, "tls.crt")
	}
	if keyFile == "" {
		keyFile = filepath.Join(cfg.DataDir, "tls.key")
	}
	if _, err := os.Stat(certFile); err == nil {
		return certFile, keyFile, nil
	}
	slog.Info("generating self-signed TLS certificate")
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "mmapi"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	os.MkdirAll(filepath.Dir(certFile), 0o755)
	certOut, _ := os.Create(certFile)
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	certOut.Close()
	keyBytes, _ := x509.MarshalECPrivateKey(key)
	keyOut, _ := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	keyOut.Close()
	return certFile, keyFile, nil
}
