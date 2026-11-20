package services

import (
	"strings"
	"testing"

	"github.com/webteractive/l4win/cli/internal/sites"
	"github.com/webteractive/l4win/cli/internal/version"
)

func TestCaddyfile(t *testing.T) {
	ups := []Upstream{
		{
			Site:  sites.Site{Name: "shop", Path: `C:\code\My Shop`, Root: `C:\code\My Shop\public`},
			PHP:   version.Version{8, 4, 1},
			Ports: []int{9100, 9101},
		},
		{
			Site:  sites.Site{Name: "legacy", Path: `C:\code\legacy`, Root: `C:\code\legacy\public`},
			PHP:   version.Version{8, 2, 3},
			Ports: []int{9102},
		},
	}
	got := Caddyfile(`C:\l4win\data\web`, ups, true)
	for _, want := range []string{
		"\tadmin off\n",
		`storage file_system "C:/l4win/data/web"`,
		"\tlocal_certs\n",
		"\tskip_install_trust\n",
		"shop.localhost {",
		`root * "C:/code/My Shop/public"`, // forward slashes, quoted for spaces
		"php_fastcgi 127.0.0.1:9100 127.0.0.1:9101 {",
		"lb_policy least_conn",
		"legacy.localhost {",
		"php_fastcgi 127.0.0.1:9102 {",
		"file_server",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Caddyfile is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "disable_redirects") {
		t.Error("redirects should be on when port 80 is free")
	}
	if !strings.Contains(Caddyfile(`C:\x`, ups, false), "auto_https disable_redirects") {
		t.Error("redirects should be off when port 80 is taken")
	}
}
