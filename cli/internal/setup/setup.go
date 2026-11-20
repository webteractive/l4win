// Package setup installs l4win's shims and puts them on the user's PATH.
package setup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/webteractive/l4win/cli/internal/home"
)

// Shims are the commands l4win answers to besides its own name. Each is a
// copy (hard link when possible) of l4win.exe that checks its own name.
var Shims = []string{"php", "composer", "node", "npm", "npx"}

// InstallShims copies the running l4win.exe into the bin folder and creates
// the shims next to it.
func InstallShims() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, _ = filepath.EvalSymlinks(self)
	bin := home.Bin()
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return err
	}
	cleanupOld(bin)

	target := filepath.Join(bin, "l4win.exe")
	if !samePath(self, target) {
		if err := replace(target, func(dst string) error { return copyFile(self, dst) }); err != nil {
			return fmt.Errorf("installing l4win.exe: %w", err)
		}
	}
	for _, name := range Shims {
		dst := filepath.Join(bin, name+".exe")
		err := replace(dst, func(dst string) error {
			if os.Link(target, dst) == nil {
				return nil
			}
			return copyFile(target, dst)
		})
		if err != nil {
			return fmt.Errorf("creating %s.exe: %w", name, err)
		}
	}
	return nil
}

// replace writes path via write. A running .exe can't be deleted on
// Windows but can be renamed, so an existing file in use is moved aside
// and removed by a later setup.
func replace(path string, write func(string) error) error {
	if home.Exists(path) {
		if err := os.Remove(path); err != nil {
			aside := fmt.Sprintf("%s.old-%d", path, os.Getpid())
			if err := os.Rename(path, aside); err != nil {
				return err
			}
		}
	}
	return write(path)
}

func cleanupOld(bin string) {
	old, _ := filepath.Glob(filepath.Join(bin, "*.old-*"))
	for _, f := range old {
		os.Remove(f) // still in use: try again next time
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func samePath(a, b string) bool {
	a, _ = filepath.Abs(a)
	b, _ = filepath.Abs(b)
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// Conflict is a command that resolves to something other than l4win's shim.
type Conflict struct {
	Command string
	Path    string // what runs instead
}

// Conflicts checks, for each shim, whether something earlier on the PATH
// that new terminals will get would run instead.
func Conflicts() []Conflict {
	var out []Conflict
	bin := home.Bin()
	dirs := filepath.SplitList(EffectivePath())
	for _, name := range Shims {
		for _, dir := range dirs {
			dir = strings.Trim(dir, `"`)
			if dir == "" {
				continue
			}
			found := ""
			for _, ext := range []string{".exe", ".bat", ".cmd"} {
				if p := filepath.Join(dir, name+ext); home.Exists(p) {
					found = p
					break
				}
			}
			if found == "" {
				continue
			}
			if !samePath(dir, bin) {
				out = append(out, Conflict{Command: name, Path: found})
			}
			break
		}
	}
	return out
}
