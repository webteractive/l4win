// Package node installs the official Node.js Windows builds, which Laravel
// uses for Vite (`npm run dev`, `npm run build`).
package node

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/webteractive/l4win/cli/internal/fetch"
	"github.com/webteractive/l4win/cli/internal/home"
	"github.com/webteractive/l4win/cli/internal/version"
)

const dist = "https://nodejs.org/dist/"

// Release is one Node.js version from the official index.
type Release struct {
	Version version.Version
	LTS     bool
}

var releasesCache []Release

// Releases returns the Node versions that have a Windows x64 zip, newest first.
func Releases() ([]Release, error) {
	if releasesCache != nil {
		return releasesCache, nil
	}
	var index []struct {
		Version string   `json:"version"`
		Files   []string `json:"files"`
		LTS     any      `json:"lts"` // false, or the LTS codename
	}
	if err := fetch.JSON(dist+"index.json", &index); err != nil {
		return nil, fmt.Errorf("reading the Node.js release list: %w", err)
	}
	var out []Release
	for _, r := range index {
		if strings.Contains(r.Version, "-") { // alpha, rc, nightly
			continue
		}
		v, err := version.Parse(r.Version)
		if err != nil {
			continue
		}
		for _, f := range r.Files {
			if f == "win-x64-zip" {
				lts, _ := r.LTS.(string)
				out = append(out, Release{Version: v, LTS: lts != ""})
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version.Compare(out[j].Version) > 0 })
	releasesCache = out
	return out, nil
}

// Pick returns the newest release matching want: "" or "lts" for the
// newest LTS, "latest" for the newest release, or a version ("24", "24.22",
// "^22").
func Pick(want string) (Release, error) {
	releases, err := Releases()
	if err != nil {
		return Release{}, err
	}
	want = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(want)), "v")
	switch {
	case want == "" || want == "lts" || strings.HasPrefix(want, "lts/"):
		for _, r := range releases {
			if r.LTS {
				return r, nil
			}
		}
	case want == "latest" || want == "node" || want == "current":
		return releases[0], nil
	default:
		c, err := version.ParseConstraint(want)
		if err != nil {
			return Release{}, err
		}
		for _, r := range releases {
			if c.Check(r.Version) {
				return r, nil
			}
		}
	}
	return Release{}, fmt.Errorf("no Node.js release matches %q", want)
}

// Root holds all installed Node versions.
func Root() string { return home.Path("node") }

// Dir is one version's folder.
func Dir(v version.Version) string { return filepath.Join(Root(), v.String()) }

// Exe is node.exe of one version.
func Exe(v version.Version) string { return filepath.Join(Dir(v), "node.exe") }

// CLI returns the path of npm-cli.js or npx-cli.js for a version.
func CLI(v version.Version, tool string) string {
	return filepath.Join(Dir(v), "node_modules", "npm", "bin", tool+"-cli.js")
}

// Installed lists installed versions, newest first.
func Installed() []version.Version {
	entries, _ := os.ReadDir(Root())
	var out []version.Version
	for _, e := range entries {
		v, err := version.Parse(e.Name())
		if e.IsDir() && err == nil && v.String() == e.Name() && home.Exists(Exe(v)) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Compare(out[j]) > 0 })
	return out
}

// Install installs the newest release matching want and removes older
// releases of the same major version.
func Install(want string) (version.Version, error) {
	r, err := Pick(want)
	if err != nil {
		return version.Version{}, err
	}
	v := r.Version
	if home.Exists(Exe(v)) {
		fmt.Fprintf(os.Stderr, "Node.js %s is already installed.\n", v)
		return v, nil
	}

	name := fmt.Sprintf("node-v%s-win-x64.zip", v)
	sums, err := fetch.Text(fmt.Sprintf("%sv%s/SHASUMS256.txt", dist, v))
	if err != nil {
		return version.Version{}, fmt.Errorf("reading Node.js checksums: %w", err)
	}
	sha := ""
	for _, line := range strings.Split(sums, "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] == name {
			sha = f[0]
		}
	}
	if sha == "" {
		return version.Version{}, fmt.Errorf("no checksum published for %s", name)
	}

	fmt.Fprintf(os.Stderr, "Installing Node.js %s\n", v)
	if err := fetch.Zip(fmt.Sprintf("%sv%s/%s", dist, v, name), sha, Dir(v), name); err != nil {
		return version.Version{}, err
	}
	for _, old := range Installed() {
		if old[0] == v[0] && old != v && os.RemoveAll(Dir(old)) == nil {
			fmt.Fprintf(os.Stderr, "Removed Node.js %s (replaced by %s).\n", old, v)
		}
	}
	fmt.Fprintf(os.Stderr, "Node.js %s is ready.\n", v)
	return v, nil
}

// Uninstall removes installed versions matching want.
func Uninstall(want string) ([]version.Version, error) {
	c, err := version.ParseConstraint(strings.TrimPrefix(want, "v"))
	if err != nil {
		return nil, err
	}
	var removed []version.Version
	for _, v := range Installed() {
		if c.Check(v) {
			if err := os.RemoveAll(Dir(v)); err != nil {
				return removed, fmt.Errorf("removing Node.js %s: %w (is it still running?)", v, err)
			}
			removed = append(removed, v)
		}
	}
	return removed, nil
}

// Resolve picks the Node version for dir: .nvmrc or .node-version (walking
// up), then the default, then the newest installed. Installs one if needed.
func Resolve(dir string) (version.Version, error) {
	installed := Installed()
	want, file := findWanted(dir)
	if want == "" {
		want = home.LoadConfig().Node
	}

	if want != "" && !strings.HasPrefix(strings.ToLower(want), "lts") && want != "node" && want != "latest" {
		if c, err := version.ParseConstraint(strings.TrimPrefix(want, "v")); err == nil {
			for _, v := range installed {
				if c.Check(v) {
					return v, nil
				}
			}
			if file != "" {
				fmt.Fprintf(os.Stderr, "l4win: %s asks for Node.js %s; installing it.\n", file, want)
			}
			return Install(want)
		}
	}
	if len(installed) > 0 {
		return installed[0], nil
	}
	fmt.Fprintln(os.Stderr, "l4win: Node.js isn't installed yet; installing the latest LTS.")
	return Install("lts")
}

func findWanted(dir string) (string, string) {
	if want := strings.TrimSpace(os.Getenv("L4WIN_NODE")); want != "" {
		return want, "L4WIN_NODE"
	}
	dir, _ = filepath.Abs(dir)
	for {
		for _, name := range []string{".nvmrc", ".node-version"} {
			file := filepath.Join(dir, name)
			if data, err := os.ReadFile(file); err == nil {
				if want := strings.TrimSpace(string(data)); want != "" {
					return want, file
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}
