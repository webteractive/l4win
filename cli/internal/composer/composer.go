// Package composer installs Composer's official phar.
package composer

import (
	"fmt"
	"os"
	"strings"

	"github.com/webteractive/l4win/cli/internal/fetch"
	"github.com/webteractive/l4win/cli/internal/home"
)

const base = "https://getcomposer.org/download/latest-stable/composer.phar"

// Phar is the installed composer.phar.
func Phar() string { return home.Path("composer", "composer.phar") }

// Installed reports whether Composer is installed.
func Installed() bool { return home.Exists(Phar()) }

// Install downloads the latest stable Composer, checked against its
// published SHA-256. Later updates go through `composer self-update`.
func Install() error {
	if Installed() {
		fmt.Fprintln(os.Stderr, "Composer is already installed (update it with `composer self-update`).")
		return nil
	}
	sum, err := fetch.Text(base + ".sha256sum")
	if err != nil {
		return fmt.Errorf("reading Composer's checksum: %w", err)
	}
	fields := strings.Fields(sum)
	if len(fields) == 0 {
		return fmt.Errorf("empty checksum at %s.sha256sum", base)
	}
	fmt.Fprintln(os.Stderr, "Installing Composer")
	if err := fetch.File(base, Phar(), fields[0], "composer.phar"); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Composer is ready.")
	return nil
}
