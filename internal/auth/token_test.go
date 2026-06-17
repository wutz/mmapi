package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wutz/mmapi/internal/config"
)

func newTestStore(t *testing.T) *TokenStore {
	t.Helper()
	cfg := &config.Config{
		DataDir: t.TempDir(),
	}
	return NewTokenStore(cfg)
}

func TestCreateAndValidateToken(t *testing.T) {
	store := newTestStore(t)

	token, err := store.Create([]string{"gpfs0"}, []string{"fileset-a"})
	if err != nil {
		t.Fatal(err)
	}
	if token.ID == "" || token.Secret == "" {
		t.Fatal("expected non-empty ID and Secret")
	}
	if token.Secret[:6] != "mmapi_" {
		t.Errorf("expected secret prefix 'mmapi_', got %q", token.Secret[:6])
	}

	got, ok := store.Validate(token.Secret)
	if !ok {
		t.Fatal("expected token to be valid")
	}
	if got.ID != token.ID {
		t.Errorf("expected ID %q, got %q", token.ID, got.ID)
	}
}

func TestInvalidToken(t *testing.T) {
	store := newTestStore(t)
	_, ok := store.Validate("invalid")
	if ok {
		t.Fatal("expected invalid token")
	}
}

func TestDeleteToken(t *testing.T) {
	store := newTestStore(t)
	token, _ := store.Create([]string{"gpfs0"}, nil)
	if err := store.Delete(token.ID); err != nil {
		t.Fatal(err)
	}
	_, ok := store.Validate(token.Secret)
	if ok {
		t.Fatal("expected token to be deleted")
	}
}

func TestCheckAccessFS(t *testing.T) {
	store := newTestStore(t)
	token, _ := store.Create([]string{"gpfs0", "gpfs1"}, nil)

	if err := store.CheckAccess(token, "gpfs0", ""); err != nil {
		t.Errorf("expected access to gpfs0: %v", err)
	}
	if err := store.CheckAccess(token, "gpfs1", ""); err != nil {
		t.Errorf("expected access to gpfs1: %v", err)
	}
	if err := store.CheckAccess(token, "gpfs2", ""); err == nil {
		t.Error("expected access denied for gpfs2")
	}
}

func TestCheckAccessFileset(t *testing.T) {
	store := newTestStore(t)
	token, _ := store.Create([]string{"gpfs0"}, []string{"fileset-a", "fileset-b"})

	if err := store.CheckAccess(token, "gpfs0", "fileset-a"); err != nil {
		t.Errorf("expected access to fileset-a: %v", err)
	}
	if err := store.CheckAccess(token, "gpfs0", "fileset-c"); err == nil {
		t.Error("expected access denied for fileset-c")
	}
}

func TestTokenPersistence(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir}

	store1 := NewTokenStore(cfg)
	token, _ := store1.Create([]string{"gpfs0"}, []string{"fs-a"})

	path := filepath.Join(dir, "tokens.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatal("expected tokens.json to exist")
	}

	store2 := NewTokenStore(cfg)
	got, ok := store2.Validate(token.Secret)
	if !ok {
		t.Fatal("expected token to persist across store instances")
	}
	if got.ID != token.ID {
		t.Errorf("expected ID %q, got %q", token.ID, got.ID)
	}
}

func TestListTokens(t *testing.T) {
	store := newTestStore(t)
	store.Create([]string{"gpfs0"}, nil)
	store.Create([]string{"gpfs1"}, nil)

	tokens := store.List()
	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(tokens))
	}
}
