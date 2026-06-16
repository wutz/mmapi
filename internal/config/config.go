package config

import (
	"encoding/json"
	"os"
)

type Mode string

const (
	ModeMultiFS      Mode = "multi-fs"
	ModeMultiFileset Mode = "multi-fileset"
)

type Config struct {
	Port    int    `json:"port"`
	Mode    Mode   `json:"mode"`
	Device  string `json:"device"`
	DataDir string `json:"dataDir"`
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:    8080,
		Mode:    ModeMultiFileset,
		Device:  "gpfs0",
		DataDir: "/var/lib/mmapi",
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
