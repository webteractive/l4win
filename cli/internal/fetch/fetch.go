// Package fetch downloads files, checks their SHA-256 and unpacks zips.
package fetch

import (
	"archive/zip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/webteractive/l4win/cli/internal/home"
)

// Version is sent in the User-Agent; main sets it at startup.
var Version = "dev"

var client = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ResponseHeaderTimeout: 60 * time.Second,
		TLSHandshakeTimeout:   30 * time.Second,
	},
}

func get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "l4win/"+Version)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp, nil
}

// Text fetches a small text resource.
func Text(url string) (string, error) {
	resp, err := get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	return string(data), err
}

// JSON fetches url and decodes it into v.
func JSON(url string, v any) error {
	resp, err := get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// File downloads url to dest, showing progress on stderr under label.
// If sha is not empty the download must match it, or dest is not written.
// sha is a hex SHA-256 (64 characters) or SHA-512 (128, as .NET publishes).
func File(url, dest, sha, label string) error {
	resp, err := get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	var hasher hash.Hash = sha256.New()
	if len(sha) == 128 {
		hasher = sha512.New()
	}
	p := &progress{label: label, total: resp.ContentLength}
	_, err = io.Copy(io.MultiWriter(tmp, hasher, p), resp.Body)
	p.done()
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}

	if sha != "" {
		if got := hex.EncodeToString(hasher.Sum(nil)); !strings.EqualFold(got, sha) {
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", url, sha, got)
		}
	}
	os.Remove(dest)
	return os.Rename(tmp.Name(), dest)
}

// Zip downloads a zip, verifies it and unpacks it into dir, leaving out the
// top-level folders named in skip. When the zip has a single top-level
// folder (as Node's does) its contents are unpacked directly into dir, and
// skip is relative to that folder. dir appears only once everything is
// unpacked.
func Zip(url, sha, dir, label string, skip ...string) error {
	cache := home.Path("cache")
	archive := filepath.Join(cache, filepath.Base(url))
	if err := File(url, archive, sha, label); err != nil {
		return err
	}
	defer os.Remove(archive)

	staging := dir + ".partial"
	os.RemoveAll(staging)
	if err := unzip(archive, staging, skip); err != nil {
		os.RemoveAll(staging)
		return err
	}
	os.RemoveAll(dir)
	if err := os.Rename(staging, dir); err != nil {
		os.RemoveAll(staging)
		return err
	}
	return nil
}

// ZipMember downloads a zip, verifies it, and writes only the named file
// (a path inside the zip, e.g. "php_redis.dll") to dest.
func ZipMember(url, sha, member, dest, label string) error {
	archive := filepath.Join(home.Path("cache"), filepath.Base(url))
	if err := File(url, archive, sha, label); err != nil {
		return err
	}
	defer os.Remove(archive)

	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if strings.EqualFold(strings.ReplaceAll(f.Name, "\\", "/"), member) {
			tmp := dest + ".partial"
			if err := extract(f, tmp); err != nil {
				os.Remove(tmp)
				return err
			}
			os.Remove(dest)
			return os.Rename(tmp, dest)
		}
	}
	return fmt.Errorf("%s has no %s", filepath.Base(url), member)
}

func unzip(archive, dest string, skip []string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()

	strip := commonRoot(r.File)
	root, err := filepath.Abs(dest)
	if err != nil {
		return err
	}

	for _, f := range r.File {
		name := strings.TrimPrefix(strings.ReplaceAll(f.Name, "\\", "/"), strip)
		if name == "" || skipped(name, skip) {
			continue
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		// Refuse entries that would land outside dest ("zip slip").
		if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path in archive: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := extract(f, target); err != nil {
			return err
		}
	}
	return nil
}

func extract(f *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}

// commonRoot returns "name/" when every entry sits under one folder.
func commonRoot(files []*zip.File) string {
	root := ""
	for _, f := range files {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		i := strings.Index(name, "/")
		if i < 0 {
			return "" // a file at the top level
		}
		if root == "" {
			root = name[:i+1]
		} else if name[:i+1] != root {
			return ""
		}
	}
	return root
}

type progress struct {
	label   string
	total   int64
	written int64
	last    time.Time
}

func (p *progress) Write(b []byte) (int, error) {
	p.written += int64(len(b))
	if time.Since(p.last) > 200*time.Millisecond {
		p.last = time.Now()
		p.print()
	}
	return len(b), nil
}

func (p *progress) print() {
	if p.total > 0 {
		fmt.Fprintf(os.Stderr, "\r  %s  %3d%%  %s / %s ", p.label, p.written*100/p.total, mb(p.written), mb(p.total))
	} else {
		fmt.Fprintf(os.Stderr, "\r  %s  %s ", p.label, mb(p.written))
	}
}

func (p *progress) done() {
	p.print()
	fmt.Fprintln(os.Stderr)
}

func mb(n int64) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }

// skipped reports whether name is one of the top-level folders in skip.
func skipped(name string, skip []string) bool {
	for _, s := range skip {
		if name == s || strings.HasPrefix(name, s+"/") {
			return true
		}
	}
	return false
}
