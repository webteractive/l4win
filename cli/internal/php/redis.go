package php

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/webteractive/l4win/cli/internal/fetch"
	"github.com/webteractive/l4win/cli/internal/home"
	"github.com/webteractive/l4win/cli/internal/version"
)

// phpredis is the Redis extension Laravel uses by default
// (REDIS_CLIENT=phpredis). PHP for Windows doesn't include it; the PHP team
// publishes builds per PHP version but no checksums, so the SHA-256 of each
// official build is pinned here and updated with l4win releases.
const phpredisVersion = "6.3.0"

var phpredisBuilds = map[string]struct{ file, sha string }{
	"8.1": {"php_redis-6.3.0-8.1-nts-vs16-x64.zip", "952ec845408e343f273eab109e62a9c7732178d93985120736da4a171d41b9b0"},
	"8.2": {"php_redis-6.3.0-8.2-nts-vs16-x64.zip", "b2b730b99b97352212b338c01f7dde577857e31c5f8e1d011ec2871e63f8f87c"},
	"8.3": {"php_redis-6.3.0-8.3-nts-vs16-x64.zip", "519fc1bdf54323d3ab08443c66f77e783d314c13b532e27cc8b390def7f81b60"},
	"8.4": {"php_redis-6.3.0-8.4-nts-vs17-x64.zip", "6db881ed172703962002d7e02dacc8ddb3f789904c648fee3e6ceec1319679cd"},
	"8.5": {"php_redis-6.3.0-8.5-nts-vs17-x64.zip", "481d6d1af45060ab41af6abe250faa270276fc47a493badc2e178024cdf6e255"},
}

func redisDLL(v version.Version) string { return filepath.Join(Dir(v), "ext", "php_redis.dll") }

// HasRedis reports whether the redis extension is installed for v.
func HasRedis(v version.Version) bool { return home.Exists(redisDLL(v)) }

// InstallRedis adds the redis extension to an installed PHP and enables it.
func InstallRedis(v version.Version) error {
	if HasRedis(v) {
		return nil
	}
	build, ok := phpredisBuilds[v.Minor()]
	if !ok {
		return fmt.Errorf("no redis extension build is known for PHP %s; use REDIS_CLIENT=predis (composer require predis/predis)", v.Minor())
	}
	fmt.Fprintf(os.Stderr, "Installing the redis extension %s for PHP %s\n", phpredisVersion, v)
	url := "https://downloads.php.net/~windows/pecl/releases/redis/" + phpredisVersion + "/" + build.file
	if err := fetch.ZipMember(url, build.sha, "php_redis.dll", redisDLL(v), build.file); err != nil {
		return err
	}
	// php.ini enables every extension in Extensions whose DLL is present.
	return writeIni(Dir(v), v)
}
