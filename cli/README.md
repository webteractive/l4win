# l4win.exe

The manager half of l4win: a single Windows executable that installs and runs
everything a Laravel project needs, so there's no Docker, WSL, Herd or XAMPP.
Users never need Go; this folder is only for building it.

## What it does

- Downloads the official builds into `%LOCALAPPDATA%\l4win` (override with
  `L4WIN_HOME`), checking each against its published SHA-256:
  - PHP: non-thread-safe x64 builds from windows.php.net, with a `php.ini`
    set up for Laravel (extensions, CA bundle, memory limit)
  - Composer: `getcomposer.org/download/latest-stable`
  - Node.js: `nodejs.org/dist` (latest LTS by default)
  - CA bundle: `curl.se/ca/cacert.pem` (Windows PHP has none)
- `l4win setup` copies itself to `%LOCALAPPDATA%\l4win\bin` along with the shims
  `php.exe`, `composer.exe`, `node.exe`, `npm.exe` and `npx.exe` (hard links to
  `l4win.exe` that check their own name), and puts that folder first on the
  user PATH. No admin rights needed.
- Each shim picks the project's version (see below), installs it if missing, and
  runs the real program in a Windows Job Object. If the shim is killed, the
  program and everything it started stop too, so a killed `artisan serve` or
  Vite doesn't leave orphan processes behind.

- `l4win up` starts the services the project's `.env` needs, shared by all
  projects:

  | Service | When | Source | Data |
  | --- | --- | --- | --- |
  | MySQL 8.4 | `DB_CONNECTION=mysql` | cdn.mysql.com (version and SHA-256 pinned in `internal/services/mysql.go`, since MySQL has no index) | `data\mysql-8.4` |
  | MariaDB (newest LTS) | `DB_CONNECTION=mariadb` | downloads.mariadb.org REST API | `data\mariadb-<series>` |
  | PostgreSQL 18 | `DB_CONNECTION=pgsql` | EDB's Windows binaries (linked from postgresql.org; version and SHA-256 pinned in `internal/services/postgres.go`, since EDB publishes no checksums). pgAdmin, StackBuilder, docs and headers are skipped while unpacking | `data\postgres-18` |
  | Redis (Garnet) | a cache, queue, session or broadcast driver set to `redis` | GitHub releases of microsoft/garnet (asset digest) | `data\redis` (checkpoints) |
  | Mailpit | `MAIL_MAILER=smtp` to 127.0.0.1 | GitHub releases (asset digest) | `data\mailpit` |

  Garnet is a framework-dependent .NET app: l4win downloads the matching
  runtime (channel read from the build's runtimeconfig) from Microsoft's release
  metadata, checked against its SHA-512, into `dotnet\<version>`, and starts
  Garnet with `DOTNET_ROOT` pointing there. It runs with `--lua` (Laravel's
  queue and lock scripts), `--checkpointdir` + `--recover`, and 512 MB of log
  memory instead of the 16 GB default. Garnet has no SHUTDOWN, so `down` sends
  `SAVE` and then ends it. The `redis` PHP extension (phpredis 6.3.0) comes from
  the PHP team's PECL builds, which publish no checksums; their SHA-256 is
  pinned in `internal/php/redis.go`. It's installed with every PHP and
  enabled through php.ini.

  They run hidden, in their own process group and outside any job, so they
  outlive the terminal until `l4win down`. State is in `run\<name>.json`
  (PID plus executable path, so a reused PID isn't mistaken for the service).
  Databases are stopped cleanly (`mysqladmin shutdown`, `pg_ctl stop -m fast`),
  falling back to a kill after 30 s. MySQL's `root` has no password, like
  Laravel's default `.env`; other `DB_USERNAME`s are created for `localhost` and
  `127.0.0.1` with rights on `DB_DATABASE` only. PostgreSQL is initialized with
  superuser `postgres`, UTF-8, no locale, and trust authentication (it only
  listens on 127.0.0.1); `DB_USERNAME` becomes a login role that owns
  `DB_DATABASE`. Data folders are per major version. A busy port is reported with the program holding it.
  `.env` hosts that aren't this machine (Sail's `mysql`, `mailpit`) are
  pointed out instead of started.

- Sites (`park`, `link`) are served at `https://NAME.localhost` by the `web`
  service: official Caddy (GitHub release, SHA-512 from its checksums file)
  on 443, with HTTP→HTTPS redirects on 80 when it's free. l4win writes
  `etc\Caddyfile` on each start (`admin off`, `local_certs`, storage in
  `data\web`, `skip_install_trust`); each site gets `php_fastcgi` with
  `least_conn` over the php-cgi workers of its PHP version
  (`WorkersPerPHP` per version, ports from 9100, `PHP_FCGI_MAX_REQUESTS=500`).
  A hidden `l4win __web-daemon` process puts itself in a kill-on-close Job
  Object, runs Caddy and the workers, and restarts any that exit (with
  backoff). Stopping the service kills it, which takes the rest down.
  `l4win trust` runs `certutil -user -addstore Root` on Caddy's root
  certificate. Site changes restart the web server if it's running.

Version selection:

| Tool | Order |
| --- | --- |
| PHP | `L4WIN_PHP`, `.php-version`, `require.php` in `composer.json`, `l4win use php@X`, newest installed |
| Node | `L4WIN_NODE`, `.nvmrc` / `.node-version`, `l4win use node@X`, newest installed |

Constraints follow Composer's rules (`^8.2`, `~8.2`, `8.3.*`, `>=8.1 <8.4`,
`^8.2 || ^8.3`). A bare `8.3` means `8.3.*`.

Your own PHP settings go in `%LOCALAPPDATA%\l4win\etc\php\<minor>\*.ini`. They're
loaded through `PHP_INI_SCAN_DIR` and kept when PHP is updated (`php.ini` is
rewritten).

## Layout

```
main.go, commands.go, tools.go   command dispatch, shims
internal/version                 versions and Composer constraints (tested)
internal/fetch                   download, SHA-256 check, zip extraction
internal/php                     PHP releases, install, php.ini, version resolution
internal/node                    Node releases, install, version resolution
internal/composer                composer.phar
internal/proc                    foreground runner with Job Object and Ctrl+C handling
internal/setup                   shims, user PATH, PATH conflict check
internal/services                MySQL, MariaDB, Redis (Garnet), Mailpit: install, start hidden, stop
internal/dotnet                  private .NET runtime for Garnet
internal/sites                   parked folders and links → sites (tested)
internal/services/web.go         Caddy, Caddyfile (tested), php-cgi supervisor, trust
internal/project                 .env reader and which services it needs (tested)
```

## Build

Requires Go (see `go.mod`). No C compiler is needed.

```bash
cd cli
go test ./...
go build -ldflags "-X main.buildVersion=0.1.0" -o dist/l4win.exe .
```

The same commands work in PowerShell and Command Prompt when `go` is on the
PATH. With Go somewhere else, call it by its full path:

```powershell
# PowerShell
& "C:\path\to\go\bin\go.exe" build -o dist\l4win.exe .
```

```cmd
:: Command Prompt
"C:\path\to\go\bin\go.exe" build -o dist\l4win.exe .
```

To try it without touching your real setup (PowerShell):

```powershell
$env:L4WIN_HOME = "$env:TEMP\l4win-test"
.\dist\l4win.exe setup --no-path
& "$env:TEMP\l4win-test\bin\php.exe" -v
```

## Release

Push a tag `cli-vX.Y.Z`. The `l4win.exe` workflow tests, builds and attaches
`l4win.exe` and its `.sha256` to a GitHub release.
