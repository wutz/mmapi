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
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/wutz/mmapi/internal/api"
	"github.com/wutz/mmapi/internal/auth"
	"github.com/wutz/mmapi/internal/config"
	"github.com/wutz/mmapi/internal/gpfs"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Create executor and token store early for middleware
	executor := gpfs.NewExecutor()
	tokenStore := auth.NewTokenStore(cfg)

	router := chi.NewMux()

	// Middleware: intercept directory creation before Huma processes the request
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" && strings.Contains(r.URL.Path, "/directory/") &&
				strings.HasPrefix(r.URL.Path, "/scalemgmt/v2/filesystems/") {
				// Extract filesystem and relative path
				parts := strings.SplitN(r.URL.Path[len("/scalemgmt/v2/filesystems/"):], "/directory/", 2)
				if len(parts) == 2 {
					filesystem := parts[0]
					relativePath := parts[1]
					if decoded, err := url.PathUnescape(relativePath); err == nil {
						relativePath = decoded
					}

					if err := executor.CreateDirectory(r.Context(), filesystem, relativePath); err != nil {
						if !strings.Contains(err.Error(), "File exists") && !strings.Contains(err.Error(), "already exists") {
							slog.Error("create directory failed", "error", err)
						}
					}

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(202)
					json.NewEncoder(w).Encode(map[string]any{
						"status": map[string]any{"code": 202, "message": "created"},
						"jobs":   []any{map[string]any{"jobId": 1, "status": "COMPLETED"}},
					})
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	})

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.RequestID)

	humaConfig := huma.DefaultConfig("mmapi", "1.0.0")
	humaAPI := humachi.New(router, humaConfig)

	authMiddleware := auth.Middleware(tokenStore)

	api.RegisterRoutes(humaAPI, cfg, tokenStore, authMiddleware)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: router,
	}

	go func() {
		if cfg.TLS {
			certFile, keyFile, err := ensureTLSCerts(cfg)
			if err != nil {
				slog.Error("failed to setup TLS", "error", err)
				os.Exit(1)
			}
			slog.Info("starting HTTPS server", "port", cfg.Port)
			if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
				slog.Error("server error", "error", err)
				os.Exit(1)
			}
		} else {
			slog.Info("starting HTTP server", "port", cfg.Port)
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

	certOut, err := os.Create(certFile)
	if err != nil {
		return "", "", err
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	certOut.Close()

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", "", err
	}
	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", "", err
	}
	pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	keyOut.Close()

	return certFile, keyFile, nil
}
