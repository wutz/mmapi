package config

import (
	"encoding/json"
	"os"
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
	AdminToken  string `json:"adminToken"`
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

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
