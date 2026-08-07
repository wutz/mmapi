package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port        int    `json:"port"`
	DataDir     string `json:"dataDir"`
	TLS         bool   `json:"tls"`
	CertFile    string `json:"certFile"`
	KeyFile     string `json:"keyFile"`
	GuiURL      string `json:"guiUrl"`
	GuiUsername string `json:"guiUsername"`
	GuiPassword string `json:"guiPassword"`
	// AdminToken is the bearer token protecting the token management API. It is
	// required: an mmapi whose management API is unauthenticated would let any
	// caller mint a token for any filesystem, which is the whole boundary the
	// proxy exists to enforce.
	AdminToken string `json:"adminToken"`
	// GuiVerifyTLS controls whether the upstream GPFS GUI TLS certificate is
	// verified. Defaults to false for compatibility with self-signed GUI certs;
	// enable in trusted environments to prevent man-in-the-middle attacks.
	GuiVerifyTLS bool `json:"guiVerifyTLS"`
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:    8443,
		DataDir: "/var/lib/mmapi",
		TLS:     true,
	}

	path := os.Getenv("MMAPI_CONFIG")
	if path == "" {
		path = "/etc/mmapi/config.json"
	}

	// A missing file is an error rather than a fallback to defaults: the
	// defaults carry no GUI credentials, so the server they would start cannot
	// reach the GUI and cannot authenticate anyone.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return cfg, nil
}

// ErrPlaceholderSecret reports a credential left at the value the sample
// configuration ships with.
var ErrPlaceholderSecret = errors.New("credential still set to its placeholder value")

// placeholder marks the sample values in deploy/config*.json. A deployment that
// keeps one is running with a credential published in the repository.
const placeholder = "CHANGE_ME"

// Validate reports configuration that mmapi cannot safely run with. Every
// credential is mandatory: the proxy's only job is to stand between a tenant
// token and the GUI's admin account, and each missing field removes one half of
// that. Placeholder values are rejected for the same reason a missing one is —
// they are public.
func (c *Config) Validate() error {
	required := []struct {
		field string
		value string
	}{
		{"guiUrl", c.GuiURL},
		{"guiUsername", c.GuiUsername},
		{"guiPassword", c.GuiPassword},
		{"adminToken", c.AdminToken},
	}
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			return fmt.Errorf("%s is required", r.field)
		}
		if strings.Contains(r.value, placeholder) {
			return fmt.Errorf("%s: %w", r.field, ErrPlaceholderSecret)
		}
	}
	return nil
}
