package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig writes a config file and points Load at it.
func writeConfig(t *testing.T, body string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MMAPI_CONFIG", path)
}

const validConfig = `{
  "guiUrl": "https://127.0.0.1:443",
  "guiUsername": "admin",
  "guiPassword": "s3cret",
  "adminToken": "0123456789abcdef"
}`

func TestLoadValidConfig(t *testing.T) {
	writeConfig(t, validConfig)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AdminToken != "0123456789abcdef" {
		t.Errorf("adminToken = %q", cfg.AdminToken)
	}
	// Fields the file omits keep their defaults.
	if cfg.Port != 8443 || cfg.DataDir != "/var/lib/mmapi" || !cfg.TLS {
		t.Errorf("defaults not applied: %+v", cfg)
	}
}

// An empty adminToken used to leave /api/v1/tokens unauthenticated, so anyone
// who could reach the port could mint a token for any filesystem. Loading such
// a config must fail rather than start a server with no boundary.
func TestLoadRejectsMissingCredentials(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"no adminToken", `{"guiUrl":"https://h","guiUsername":"admin","guiPassword":"p"}`},
		{"blank adminToken", `{"guiUrl":"https://h","guiUsername":"admin","guiPassword":"p","adminToken":"   "}`},
		{"no guiUrl", `{"guiUsername":"admin","guiPassword":"p","adminToken":"t"}`},
		{"no guiUsername", `{"guiUrl":"https://h","guiPassword":"p","adminToken":"t"}`},
		{"no guiPassword", `{"guiUrl":"https://h","guiUsername":"admin","adminToken":"t"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeConfig(t, tc.body)
			if _, err := Load(); err == nil {
				t.Fatal("expected Load to reject the config")
			}
		})
	}
}

func TestLoadRejectsPlaceholderSecrets(t *testing.T) {
	writeConfig(t, `{
	  "guiUrl": "https://127.0.0.1:443",
	  "guiUsername": "admin",
	  "guiPassword": "CHANGE_ME_gui_password",
	  "adminToken": "CHANGE_ME_admin_token"
	}`)

	_, err := Load()
	if !errors.Is(err, ErrPlaceholderSecret) {
		t.Fatalf("expected ErrPlaceholderSecret, got %v", err)
	}
	// The message names the file, so the operator knows which one to edit.
	if !strings.Contains(err.Error(), "config.json") {
		t.Errorf("error should name the config path: %v", err)
	}
}

// The shipped samples must stay placeholders — a real credential committed here
// is a published credential.
func TestDeploySamplesAreNotUsable(t *testing.T) {
	matches, err := filepath.Glob("../../deploy/config*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("no deploy sample configs found")
	}
	for _, path := range matches {
		t.Setenv("MMAPI_CONFIG", path)
		if _, err := Load(); !errors.Is(err, ErrPlaceholderSecret) {
			t.Errorf("%s: expected placeholder credentials, got %v", path, err)
		}
	}
}

func TestLoadMissingFileIsAnError(t *testing.T) {
	t.Setenv("MMAPI_CONFIG", filepath.Join(t.TempDir(), "absent.json"))
	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail when the config file is absent")
	}
}
