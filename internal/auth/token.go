package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/wutz/mmapi/internal/config"
)

type Token struct {
	ID            string   `json:"id"`
	Secret        string   `json:"secret"`
	AllowedFS     []string `json:"allowedFs"`
	AllowedFileset []string `json:"allowedFileset,omitempty"`
}

// TokenInfo is the public view of a token returned by the management API. It
// intentionally omits the Secret so that listing tokens does not leak the
// credentials of every tenant.
type TokenInfo struct {
	ID             string   `json:"id"`
	AllowedFS      []string `json:"allowedFs"`
	AllowedFileset []string `json:"allowedFileset,omitempty"`
}

// ErrInvalidTokenRequest is returned by Create for malformed token requests
// (e.g. no allowed filesystems), which the caller can map to a 400 response.
var ErrInvalidTokenRequest = fmt.Errorf("invalid token request")

func (t *Token) Info() *TokenInfo {
	return &TokenInfo{
		ID:             t.ID,
		AllowedFS:      t.AllowedFS,
		AllowedFileset: t.AllowedFileset,
	}
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
	if len(allowedFS) == 0 {
		return nil, ErrInvalidTokenRequest
	}
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

func (ts *TokenStore) List() []*TokenInfo {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	var result []*TokenInfo
	for _, t := range ts.tokens {
		result = append(result, t.Info())
	}
	return result
}

func (ts *TokenStore) CheckAccess(token *Token, fs string, fileset string) error {
	if !contains(token.AllowedFS, fs) {
		return fmt.Errorf("access denied: filesystem %q not allowed", fs)
	}
	if fileset != "" && len(token.AllowedFileset) > 0 && !contains(token.AllowedFileset, fileset) {
		return fmt.Errorf("access denied: fileset %q not allowed", fileset)
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
	if err := os.MkdirAll(filepath.Dir(ts.path), 0o755); err != nil {
		return err
	}
	// Write to a temp file and rename for an atomic replacement so a crash
	// mid-write cannot corrupt tokens.json.
	tmp := ts.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, ts.path)
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
