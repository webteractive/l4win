// Package home locates l4win's data folder and its settings.
//
// Everything l4win installs lives in one folder, %LOCALAPPDATA%\l4win by
// default (L4WIN_HOME overrides it). Removing that folder removes l4win.
package home

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Dir returns l4win's data folder.
func Dir() string {
	if dir := os.Getenv("L4WIN_HOME"); dir != "" {
		return dir
	}
	if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
		return filepath.Join(dir, "l4win")
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, ".l4win")
}

// Path joins parts onto the data folder.
func Path(parts ...string) string {
	return filepath.Join(append([]string{Dir()}, parts...)...)
}

// Bin is the folder with l4win.exe and its shims; setup puts it on PATH.
func Bin() string { return Path("bin") }

// CABundle is Mozilla's CA bundle, used by PHP's curl and openssl.
func CABundle() string { return Path("cacert.pem") }

// Config holds the user's defaults.
type Config struct {
	PHP  string `json:"php,omitempty"`  // default PHP version, e.g. "8.4"
	Node string `json:"node,omitempty"` // default Node major, e.g. "24"

	// Sites served at https://NAME.localhost (see `l4win park` and `link`).
	Parked []string          `json:"parked,omitempty"` // folders whose Laravel apps are sites
	Links  map[string]string `json:"links,omitempty"`  // site name → project folder
}

func configPath() string { return Path("config.json") }

// LoadConfig reads config.json; a missing file gives empty defaults.
func LoadConfig() Config {
	var c Config
	data, err := os.ReadFile(configPath())
	if err == nil {
		_ = json.Unmarshal(data, &c)
	}
	return c
}

// SaveConfig writes config.json.
func SaveConfig(c Config) error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), append(data, '\n'), 0o644)
}

// Exists reports whether path exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}
