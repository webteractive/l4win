package sites

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/webteractive/l4win/cli/internal/home"
)

func TestName(t *testing.T) {
	for in, want := range map[string]string{
		"Shop":            "shop",
		"my_app 2":        "my-app-2",
		"--weird..name--": "weird-name",
		"Ünïcode":         "n-code",
		"___":             "",
	} {
		if got := Name(in); got != want {
			t.Errorf("Name(%q) = %q, want %q", in, got, want)
		}
	}
}

func makeApp(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "artisan"), []byte("<?php"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParkAndLink(t *testing.T) {
	code := t.TempDir()
	makeApp(t, filepath.Join(code, "Shop"))
	makeApp(t, filepath.Join(code, "blog"))
	os.MkdirAll(filepath.Join(code, "notes"), 0o755) // not an app
	other := filepath.Join(t.TempDir(), "api")
	makeApp(t, other)

	var cfg home.Config
	if ok, err := Park(&cfg, code); !ok || err != nil {
		t.Fatalf("Park: %v %v", ok, err)
	}
	if ok, _ := Park(&cfg, code); ok {
		t.Error("parking twice should be a no-op")
	}
	name, err := Link(&cfg, other, "")
	if err != nil || name != "api" {
		t.Fatalf("Link: %q %v", name, err)
	}
	if _, err := Link(&cfg, filepath.Join(code, "notes"), ""); err == nil {
		t.Error("linking a non-app should fail")
	}

	all := All(cfg)
	var names []string
	for _, s := range all {
		names = append(names, s.Name)
	}
	if len(all) != 3 || names[0] != "api" || names[1] != "blog" || names[2] != "shop" {
		t.Fatalf("All = %v", names)
	}
	if all[2].Root != filepath.Join(code, "Shop", "public") || all[2].URL() != "https://shop.localhost" {
		t.Errorf("shop site = %+v", all[2])
	}

	if s, ok := For(cfg, filepath.Join(code, "blog")); !ok || s.Name != "blog" {
		t.Errorf("For(blog) = %+v %v", s, ok)
	}

	// A link takes over a parked name.
	Link(&cfg, other, "shop")
	if s, _ := For(cfg, other); s.Name != "api" && s.Name != "shop" {
		t.Errorf("link override: %+v", s)
	}
	if removed := Unlink(&cfg, other); len(removed) != 2 {
		t.Errorf("Unlink by folder removed %v", removed)
	}
	if !Unpark(&cfg, code) || len(All(cfg)) != 0 {
		t.Errorf("after unpark: %v", All(cfg))
	}
}
