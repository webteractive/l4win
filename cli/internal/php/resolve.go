package php

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/webteractive/l4win/cli/internal/home"
	"github.com/webteractive/l4win/cli/internal/version"
)

// Requirement is the PHP version a folder asks for, and where it came from.
type Requirement struct {
	Constraint version.Constraint
	Source     string // ".php-version", "composer.json", "L4WIN_PHP", "default" or ""
	File       string // the file that set it
}

// Find walks up from dir to the nearest .php-version or composer.json.
// L4WIN_PHP overrides both.
func Find(dir string) Requirement {
	if want := strings.TrimSpace(os.Getenv("L4WIN_PHP")); want != "" {
		if c, err := version.ParseConstraint(want); err == nil {
			return Requirement{Constraint: c, Source: "L4WIN_PHP"}
		}
	}

	dir, _ = filepath.Abs(dir)
	for {
		if file := filepath.Join(dir, ".php-version"); home.Exists(file) {
			if data, err := os.ReadFile(file); err == nil {
				if c, err := version.ParseConstraint(strings.TrimSpace(string(data))); err == nil {
					return Requirement{Constraint: c, Source: ".php-version", File: file}
				}
			}
		}
		if file := filepath.Join(dir, "composer.json"); home.Exists(file) {
			// The project root: stop here even if it doesn't say.
			if c, ok := composerConstraint(file); ok {
				return Requirement{Constraint: c, Source: "composer.json", File: file}
			}
			return Requirement{}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Requirement{}
		}
		dir = parent
	}
}

func composerConstraint(file string) (version.Constraint, bool) {
	data, err := os.ReadFile(file)
	if err != nil {
		return version.Constraint{}, false
	}
	var manifest struct {
		Require map[string]string `json:"require"`
	}
	if json.Unmarshal(data, &manifest) != nil {
		return version.Constraint{}, false
	}
	want, ok := manifest.Require["php"]
	if !ok {
		return version.Constraint{}, false
	}
	c, err := version.ParseConstraint(want)
	return c, err == nil
}

// Resolve picks the PHP version to run in dir, installing one if no
// installed version fits. Preference: the default version if it fits,
// otherwise the newest installed version that fits.
func Resolve(dir string) (version.Version, error) {
	req := Find(dir)
	installed := Installed()
	def, hasDefault := Default(installed)

	if req.Source == "" {
		if hasDefault {
			return def, nil
		}
		fmt.Fprintln(os.Stderr, "l4win: no PHP installed yet; installing the latest.")
		return Install("")
	}

	if hasDefault && req.Constraint.Check(def) {
		return def, nil
	}
	for _, v := range installed {
		if req.Constraint.Check(v) {
			return v, nil
		}
	}

	fmt.Fprintf(os.Stderr, "l4win: %s asks for PHP %s; installing it.\n", req.describe(), req.Constraint)
	v, err := Install(req.Constraint.String())
	if err != nil {
		if hasDefault {
			fmt.Fprintf(os.Stderr, "l4win: %v\nl4win: using PHP %s instead.\n", err, def)
			return def, nil
		}
		return version.Version{}, err
	}
	return v, nil
}

// ResolveInstalled is Resolve without installing: it returns the installed
// version dir would use, or false if none fits.
func ResolveInstalled(dir string) (version.Version, bool) {
	req := Find(dir)
	installed := Installed()
	def, hasDefault := Default(installed)
	if req.Source == "" || (hasDefault && req.Constraint.Check(def)) {
		return def, hasDefault
	}
	for _, v := range installed {
		if req.Constraint.Check(v) {
			return v, true
		}
	}
	return version.Version{}, false
}

func (r Requirement) describe() string {
	if r.File != "" {
		return r.File
	}
	return r.Source
}

// Default returns the configured default version if installed, otherwise
// the newest installed version.
func Default(installed []version.Version) (version.Version, bool) {
	if len(installed) == 0 {
		return version.Version{}, false
	}
	if want := home.LoadConfig().PHP; want != "" {
		if c, err := version.ParseConstraint(want); err == nil {
			for _, v := range installed {
				if c.Check(v) {
					return v, true
				}
			}
		}
	}
	return installed[0], true
}
