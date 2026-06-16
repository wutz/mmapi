package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wutz/mmapi/internal/config"
)

type Token struct {
	ID            string   `json:"id"`
	Secret        string   `json:"secret"`
	AllowedFS     []string `json:"allowedFs"`
	AllowedFileset []string `json:"allowedFileset,omitempty"`
}

type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]*Token
	cfg    *config.Config
	path   string
}

func NewTokenStore(cfg *config.Config) *TokenStore {
	ts := &TokenStore{
		tokens: make(map[string]*Token),
		cfg:    cfg,
		path:   filepath.Join(cfg.DataDir, "tokens.json"),
	}
	ts.load()
	return ts
}

func (ts *TokenStore) Create(allowedFS []string, allowedFileset []string) (*Token, error) {
	secret, err := generateSecret()
	if err != nil {
		return nil, err
	}

	id, err := generateID()
	if err != nil {
		return nil, err
	}

	token := &Token{
		ID:            id,
		Secret:        secret,
		AllowedFS:     allowedFS,
		AllowedFileset: allowedFileset,
	}

	ts.mu.Lock()
	ts.tokens[secret] = token
	ts.mu.Unlock()

	return token, ts.save()
}

func (ts *TokenStore) Validate(secret string) (*Token, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	t, ok := ts.tokens[secret]
	return t, ok
}

func (ts *TokenStore) ValidateBasicAuth(user, pass string) (*Token, bool) {
	return ts.Validate(pass)
}

func (ts *TokenStore) ValidateToken(credentials, filesystem string) bool {
	// credentials format: "user:password"
	parts := strings.SplitN(credentials, ":", 2)
	if len(parts) < 2 {
		return false
	}
	token, ok := ts.Validate(parts[1])
	if !ok {
		return false
	}
	return contains(token.AllowedFS, filesystem)
}

func (ts *TokenStore) Delete(id string) error {
	ts.mu.Lock()
	for secret, t := range ts.tokens {
		if t.ID == id {
			delete(ts.tokens, secret)
			break
		}
	}
	ts.mu.Unlock()
	return ts.save()
}

func (ts *TokenStore) List() []*Token {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	var result []*Token
	for _, t := range ts.tokens {
		result = append(result, t)
	}
	return result
}

func (ts *TokenStore) CheckAccess(token *Token, fs string, fileset string) error {
	if !contains(token.AllowedFS, fs) {
		return fmt.Errorf("access denied: filesystem %q not allowed", fs)
	}
	if ts.cfg.Mode == config.ModeMultiFileset && fileset != "" {
		if len(token.AllowedFileset) > 0 && !contains(token.AllowedFileset, fileset) {
			return fmt.Errorf("access denied: fileset %q not allowed", fileset)
		}
	}
	return nil
}

func (ts *TokenStore) load() {
	data, err := os.ReadFile(ts.path)
	if err != nil {
		return
	}
	var tokens []*Token
	if err := json.Unmarshal(data, &tokens); err != nil {
		return
	}
	ts.mu.Lock()
	for _, t := range tokens {
		ts.tokens[t.Secret] = t
	}
	ts.mu.Unlock()
}

func (ts *TokenStore) save() error {
	ts.mu.RLock()
	var tokens []*Token
	for _, t := range ts.tokens {
		tokens = append(tokens, t)
	}
	ts.mu.RUnlock()

	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(ts.path), 0o755)
	return os.WriteFile(ts.path, data, 0o600)
}

func Middleware(store *TokenStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"missing authorization"}`, http.StatusUnauthorized)
				return
			}
			secret := strings.TrimPrefix(authHeader, "Bearer ")
			token, ok := store.Validate(secret)
			if !ok {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}
			r = r.WithContext(WithToken(r.Context(), token))
			next.ServeHTTP(w, r)
		})
	}
}

func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "mmapi_" + hex.EncodeToString(b), nil
}

func generateID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
