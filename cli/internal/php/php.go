// Package php installs the official PHP for Windows builds and picks the
// right version for a project.
package php

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/webteractive/l4win/cli/internal/fetch"
	"github.com/webteractive/l4win/cli/internal/home"
	"github.com/webteractive/l4win/cli/internal/version"
)

// ReleasesURL is the official index of the current Windows builds.
const ReleasesURL = "https://downloads.php.net/~windows/releases/releases.json"

const downloadBase = "https://downloads.php.net/~windows/releases/"

// Oldest is the oldest PHP version l4win offers (Laravel 10's minimum).
var Oldest = version.Version{8, 1, 0}

// Release is one downloadable build: the newest patch of a minor version.
type Release struct {
	Version version.Version
	Build   string // "8.4.27-1" as published
	File    string // "php-8.4.27-1-nts-Win32-vs17-x64.zip"
	SHA256  string
}

var releasesCache []Release

// Releases returns the available builds, newest first. Only non-thread-safe
// x64 builds are used: that's what the CLI and FrankenPHP's workers need.
func Releases() ([]Release, error) {
	if releasesCache != nil {
		return releasesCache, nil
	}
	var index map[string]map[string]json.RawMessage
	if err := fetch.JSON(ReleasesURL, &index); err != nil {
		return nil, fmt.Errorf("reading the PHP release list: %w", err)
	}

	var out []Release
	for _, entry := range index {
		var build string
		if raw, ok := entry["version"]; !ok || json.Unmarshal(raw, &build) != nil {
			continue
		}
		v, err := version.Parse(build)
		if err != nil || v.Compare(Oldest) < 0 {
			continue
		}
		for key, raw := range entry {
			if !strings.HasPrefix(key, "nts-") || !strings.HasSuffix(key, "-x64") {
				continue
			}
			var b struct {
				Zip struct {
					Path   string `json:"path"`
					SHA256 string `json:"sha256"`
				} `json:"zip"`
			}
			if json.Unmarshal(raw, &b) == nil && b.Zip.Path != "" {
				out = append(out, Release{Version: v, Build: build, File: b.Zip.Path, SHA256: b.Zip.SHA256})
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("the PHP release list has no Windows x64 builds")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version.Compare(out[j].Version) > 0 })
	releasesCache = out
	return out, nil
}

// Root is the folder holding all installed PHP versions.
func Root() string { return home.Path("php") }

// Dir is the install folder of one version.
func Dir(v version.Version) string { return filepath.Join(Root(), v.String()) }

// Exe is php.exe of one version.
func Exe(v version.Version) string { return filepath.Join(Dir(v), "php.exe") }

// ScanDir holds the user's own .ini files for a minor version. They're
// loaded after php.ini (via PHP_INI_SCAN_DIR) and survive upgrades.
func ScanDir(v version.Version) string { return home.Path("etc", "php", v.Minor()) }

// Installed lists installed versions, newest first.
func Installed() []version.Version {
	entries, _ := os.ReadDir(Root())
	var out []version.Version
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		v, err := version.Parse(e.Name())
		if err == nil && v.String() == e.Name() && home.Exists(Exe(v)) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Compare(out[j]) > 0 })
	return out
}

// Pick returns the newest release matching want ("8.3", "8.3.36", "^8.2"…).
// An empty want gives the newest release.
func Pick(want string) (Release, error) {
	releases, err := Releases()
	if err != nil {
		return Release{}, err
	}
	if want == "" {
		return releases[0], nil
	}
	c, err := version.ParseConstraint(want)
	if err != nil {
		return Release{}, err
	}
	for _, r := range releases {
		if c.Check(r.Version) {
			return r, nil
		}
	}
	var have []string
	for _, r := range releases {
		have = append(have, r.Version.String())
	}
	return Release{}, fmt.Errorf("no PHP build matches %q (available: %s)", want, strings.Join(have, ", "))
}

// Install installs the newest release matching want. Older patches of the
// same minor version are removed once the new one works.
func Install(want string) (version.Version, error) {
	r, err := Pick(want)
	if err != nil {
		return version.Version{}, err
	}
	if home.Exists(Exe(r.Version)) {
		fmt.Fprintf(os.Stderr, "PHP %s is already installed.\n", r.Version)
		return r.Version, nil
	}

	fmt.Fprintf(os.Stderr, "Installing PHP %s\n", r.Version)
	if err := ensureCABundle(); err != nil {
		return version.Version{}, err
	}
	dir := Dir(r.Version)
	if err := fetch.Zip(downloadBase+r.File, r.SHA256, dir, r.File); err != nil {
		return version.Version{}, err
	}
	if err := writeIni(dir, r.Version); err != nil {
		return version.Version{}, err
	}
	if err := verify(r.Version); err != nil {
		return r.Version, err
	}
	// Laravel's default Redis client; small, so every PHP gets it.
	if err := InstallRedis(r.Version); err != nil {
		fmt.Fprintf(os.Stderr, "l4win: skipped the redis extension: %v\n", err)
	}

	for _, old := range Installed() {
		if old.Minor() == r.Version.Minor() && old != r.Version {
			if os.RemoveAll(Dir(old)) == nil {
				fmt.Fprintf(os.Stderr, "Removed PHP %s (replaced by %s).\n", old, r.Version)
			}
		}
	}
	fmt.Fprintf(os.Stderr, "PHP %s is ready.\n", r.Version)
	return r.Version, nil
}

// Uninstall removes every installed version matching want.
func Uninstall(want string) ([]version.Version, error) {
	c, err := version.ParseConstraint(want)
	if err != nil {
		return nil, err
	}
	var removed []version.Version
	for _, v := range Installed() {
		if c.Check(v) {
			if err := os.RemoveAll(Dir(v)); err != nil {
				return removed, fmt.Errorf("removing PHP %s: %w (is it still running?)", v, err)
			}
			removed = append(removed, v)
		}
	}
	return removed, nil
}

// verify runs php -v. The usual failure is a missing Visual C++ runtime.
func verify(v version.Version) error {
	cmd := exec.Command(Exe(v), "-v")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if MissingVCRuntime() {
		return fmt.Errorf("PHP %s needs the Microsoft Visual C++ Redistributable, which isn't installed. Run `l4win install vcredist`", v)
	}
	return fmt.Errorf("PHP %s doesn't start: %v\n%s", v, err, strings.TrimSpace(string(out)))
}

// Env returns the environment variables php.exe should run with.
func Env(v version.Version) []string {
	// Keep the user's own scan folders, but drop ones l4win added for a
	// parent process (Composer running `php` again, maybe another version).
	ours := strings.ToLower(home.Path("etc", "php")) + string(os.PathSeparator)
	dirs := []string{ScanDir(v)}
	for _, d := range filepath.SplitList(os.Getenv("PHP_INI_SCAN_DIR")) {
		if d != "" && !strings.HasPrefix(strings.ToLower(filepath.Clean(d))+string(os.PathSeparator), ours) {
			dirs = append(dirs, d)
		}
	}
	return []string{"PHP_INI_SCAN_DIR=" + strings.Join(dirs, string(os.PathListSeparator))}
}
