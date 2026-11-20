package php

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/webteractive/l4win/cli/internal/fetch"
	"github.com/webteractive/l4win/cli/internal/home"
	"github.com/webteractive/l4win/cli/internal/version"
)

// Extensions enabled for Laravel when the build ships them. Others
// Laravel needs (ctype, dom, filter, hash, pcre, session, tokenizer, xml,
// bcmath…) are compiled into the Windows build.
var Extensions = []string{
	"curl", "fileinfo", "mbstring", "openssl", "intl", "gd", "zip", "exif",
	"sodium", "bz2", "sockets",
	"pdo_sqlite", "sqlite3", "pdo_mysql", "mysqli", "pdo_pgsql", "pgsql",
	"redis", // installed by l4win (see redis.go), not part of the PHP build
}

// writeIni creates php.ini from the build's php.ini-development and adds
// l4win's settings at the end, where they take precedence.
func writeIni(dir string, v version.Version) error {
	template, err := os.ReadFile(filepath.Join(dir, "php.ini-development"))
	if err != nil {
		return fmt.Errorf("the PHP build has no php.ini-development: %w", err)
	}

	ext := filepath.Join(dir, "ext")
	var b strings.Builder
	b.Write(template)
	b.WriteString("\n\n; ---------------------------------------------------------------\n")
	b.WriteString("; Added by l4win. Put your own settings in\n")
	fmt.Fprintf(&b, ";   %s\\*.ini\n", ScanDir(v))
	b.WriteString("; This file is rewritten when PHP is updated; that folder isn't.\n")
	b.WriteString("; ---------------------------------------------------------------\n")
	b.WriteString("[PHP]\n")
	fmt.Fprintf(&b, "extension_dir = \"%s\"\n", ext)
	for _, name := range Extensions {
		if home.Exists(filepath.Join(ext, "php_"+name+".dll")) {
			fmt.Fprintf(&b, "extension = %s\n", name)
		}
	}
	// OPcache is a separate DLL up to PHP 8.4 and built in from 8.5.
	if home.Exists(filepath.Join(ext, "php_opcache.dll")) {
		b.WriteString("zend_extension = opcache\n")
	}
	b.WriteString("memory_limit = 512M\n")
	b.WriteString("upload_max_filesize = 64M\n")
	b.WriteString("post_max_size = 64M\n")
	b.WriteString("date.timezone = UTC\n")
	b.WriteString("realpath_cache_size = 4096K\n")
	b.WriteString("realpath_cache_ttl = 600\n")
	// Windows PHP ships without CA certificates; without these, HTTPS
	// requests from the app (Guzzle, Http::get) fail.
	fmt.Fprintf(&b, "curl.cainfo = \"%s\"\n", home.CABundle())
	fmt.Fprintf(&b, "openssl.cafile = \"%s\"\n", home.CABundle())

	if err := os.WriteFile(filepath.Join(dir, "php.ini"), []byte(b.String()), 0o644); err != nil {
		return err
	}

	scan := ScanDir(v)
	if err := os.MkdirAll(scan, 0o755); err != nil {
		return err
	}
	readme := filepath.Join(scan, "README.txt")
	if !home.Exists(readme) {
		_ = os.WriteFile(readme, []byte(
			"Put your own PHP "+v.Minor()+" settings in .ini files in this folder, for example\n"+
				"xdebug.ini or overrides.ini. They load after php.ini and are kept when\n"+
				"l4win updates PHP.\n"), 0o644)
	}
	return nil
}

const caURL = "https://curl.se/ca/cacert.pem"

// ensureCABundle downloads Mozilla's CA bundle (as published by curl).
func ensureCABundle() error {
	if home.Exists(home.CABundle()) {
		return nil
	}
	return UpdateCABundle()
}

// UpdateCABundle downloads the current CA bundle, checked against its
// published SHA-256.
func UpdateCABundle() error {
	sum, err := fetch.Text(caURL + ".sha256")
	if err != nil {
		return fmt.Errorf("reading the CA bundle checksum: %w", err)
	}
	fields := strings.Fields(sum)
	if len(fields) == 0 {
		return fmt.Errorf("empty checksum at %s.sha256", caURL)
	}
	return fetch.File(caURL, home.CABundle(), fields[0], "cacert.pem")
}
