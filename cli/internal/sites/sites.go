// Package sites works out which projects are served at
// https://NAME.localhost: every Laravel app in a parked folder, plus
// linked projects. Browsers resolve *.localhost to this machine themselves,
// so no hosts-file changes are needed.
package sites

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/webteractive/l4win/cli/internal/home"
)

// Site is one served project.
type Site struct {
	Name   string // "shop" → https://shop.localhost
	Path   string // project folder
	Root   string // document root (public/ when it exists)
	Linked bool   // from `l4win link` rather than a parked folder
}

// URL is the site's address.
func (s Site) URL() string { return "https://" + s.Name + ".localhost" }

var unsafe = regexp.MustCompile(`[^a-z0-9-]+`)

// Name turns a folder name into a valid host name label.
func Name(folder string) string {
	name := unsafe.ReplaceAllString(strings.ToLower(folder), "-")
	name = strings.Trim(name, "-")
	if len(name) > 63 {
		name = strings.TrimRight(name[:63], "-")
	}
	return name
}

// IsLaravel reports whether dir looks like a Laravel (or other PHP) app.
func IsLaravel(dir string) bool {
	return home.Exists(filepath.Join(dir, "artisan")) || home.Exists(filepath.Join(dir, "public", "index.php"))
}

func docRoot(dir string) string {
	if public := filepath.Join(dir, "public"); home.Exists(public) {
		return public
	}
	return dir
}

// All lists the sites, sorted by name. A link wins over a parked folder
// with the same name.
func All(cfg home.Config) []Site {
	byName := map[string]Site{}
	for _, parked := range cfg.Parked {
		entries, _ := os.ReadDir(parked)
		for _, e := range entries {
			dir := filepath.Join(parked, e.Name())
			if !e.IsDir() || !IsLaravel(dir) {
				continue
			}
			if name := Name(e.Name()); name != "" {
				if _, taken := byName[name]; !taken {
					byName[name] = Site{Name: name, Path: dir, Root: docRoot(dir)}
				}
			}
		}
	}
	for name, dir := range cfg.Links {
		byName[name] = Site{Name: name, Path: dir, Root: docRoot(dir), Linked: true}
	}

	out := make([]Site, 0, len(byName))
	for _, s := range byName {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// For returns the site serving dir, if any.
func For(cfg home.Config, dir string) (Site, bool) {
	dir, _ = filepath.Abs(dir)
	for _, s := range All(cfg) {
		if strings.EqualFold(filepath.Clean(s.Path), filepath.Clean(dir)) {
			return s, true
		}
	}
	return Site{}, false
}

// Park adds a folder whose apps become sites.
func Park(cfg *home.Config, dir string) (bool, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return false, err
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return false, fmt.Errorf("%s isn't a folder", dir)
	}
	for _, p := range cfg.Parked {
		if strings.EqualFold(filepath.Clean(p), dir) {
			return false, nil
		}
	}
	cfg.Parked = append(cfg.Parked, dir)
	return true, nil
}

// Unpark removes a parked folder.
func Unpark(cfg *home.Config, dir string) bool {
	dir, _ = filepath.Abs(dir)
	for i, p := range cfg.Parked {
		if strings.EqualFold(filepath.Clean(p), dir) {
			cfg.Parked = append(cfg.Parked[:i], cfg.Parked[i+1:]...)
			return true
		}
	}
	return false
}

// Link serves dir as name (default: the folder's name).
func Link(cfg *home.Config, dir, name string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if !IsLaravel(dir) {
		return "", fmt.Errorf("%s doesn't look like a Laravel app (no artisan or public/index.php)", dir)
	}
	if name == "" {
		name = filepath.Base(dir)
	}
	clean := Name(name)
	if clean == "" {
		return "", fmt.Errorf("%q can't be a site name; use letters, digits and dashes", name)
	}
	if cfg.Links == nil {
		cfg.Links = map[string]string{}
	}
	cfg.Links[clean] = dir
	return clean, nil
}

// Unlink removes links by name, or the links pointing at dir.
func Unlink(cfg *home.Config, nameOrDir string) []string {
	var removed []string
	abs, _ := filepath.Abs(nameOrDir)
	for name, dir := range cfg.Links {
		if name == Name(nameOrDir) || strings.EqualFold(filepath.Clean(dir), abs) {
			delete(cfg.Links, name)
			removed = append(removed, name)
		}
	}
	sort.Strings(removed)
	return removed
}
