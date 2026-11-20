// Package dotnet installs Microsoft's .NET runtime privately, for services
// built on .NET (Garnet). It isn't put on the PATH or registered with
// Windows; a service gets it through DOTNET_ROOT.
package dotnet

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

// Root holds the installed runtimes, one folder per version.
func Root() string { return home.Path("dotnet") }

// Dir is one runtime's folder (what DOTNET_ROOT points to).
func Dir(ver string) string { return filepath.Join(Root(), ver) }

// Installed returns the newest installed runtime of channel ("10.0"), or "".
func Installed(channel string) string {
	entries, _ := os.ReadDir(Root())
	var found []version.Version
	for _, e := range entries {
		v, err := version.Parse(e.Name())
		if err != nil || v.String() != e.Name() || v.Minor() != channel {
			continue
		}
		if home.Exists(filepath.Join(Root(), e.Name(), "dotnet.exe")) {
			found = append(found, v)
		}
	}
	if len(found) == 0 {
		return ""
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Compare(found[j]) > 0 })
	return found[0].String()
}

// Ensure returns the folder of an installed runtime for channel,
// downloading the newest one if there's none.
func Ensure(channel string) (string, error) {
	if ver := Installed(channel); ver != "" {
		return Dir(ver), nil
	}
	return Install(channel)
}

// Install downloads the newest runtime of channel from Microsoft, checked
// against the SHA-512 in Microsoft's release metadata.
func Install(channel string) (string, error) {
	var meta struct {
		Releases []struct {
			Runtime *struct {
				Version string `json:"version"`
				Files   []struct {
					Name string `json:"name"`
					URL  string `json:"url"`
					Hash string `json:"hash"`
				} `json:"files"`
			} `json:"runtime"`
		} `json:"releases"`
	}
	url := "https://dotnetcli.blob.core.windows.net/dotnet/release-metadata/" + channel + "/releases.json"
	if err := fetch.JSON(url, &meta); err != nil {
		return "", fmt.Errorf("reading the .NET %s release list: %w", channel, err)
	}

	for _, rel := range meta.Releases { // newest first
		if rel.Runtime == nil || strings.Contains(rel.Runtime.Version, "-") {
			continue // no runtime in this release, or a preview
		}
		for _, f := range rel.Runtime.Files {
			if f.Name != "dotnet-runtime-win-x64.zip" {
				continue
			}
			ver := rel.Runtime.Version
			if home.Exists(filepath.Join(Dir(ver), "dotnet.exe")) {
				return Dir(ver), nil
			}
			fmt.Fprintf(os.Stderr, "Installing the .NET %s runtime (used only by l4win's services)\n", ver)
			if err := fetch.Zip(f.URL, f.Hash, Dir(ver), filepath.Base(f.URL)); err != nil {
				return "", err
			}
			// Older patches of the channel are no longer needed.
			entries, _ := os.ReadDir(Root())
			for _, e := range entries {
				if v, err := version.Parse(e.Name()); err == nil && v.Minor() == channel && e.Name() != ver {
					os.RemoveAll(filepath.Join(Root(), e.Name()))
				}
			}
			return Dir(ver), nil
		}
	}
	return "", fmt.Errorf("no Windows x64 runtime zip in the .NET %s release list", channel)
}

// Env returns the variables that make a .NET program use the runtime in dir
// instead of looking for one installed on the machine.
func Env(dir string) []string {
	return []string{
		"DOTNET_ROOT=" + dir,
		"DOTNET_ROOT_X64=" + dir,
		"DOTNET_CLI_TELEMETRY_OPTOUT=1",
	}
}
