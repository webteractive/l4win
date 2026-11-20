# l4win

**Laravel for Windows.** Run Laravel projects natively on Windows. Install it once per project as a dev
dependency and the Unix-only parts of the Laravel toolchain get Windows
replacements. On WSL, Linux and macOS it does nothing, so the whole team can
keep it installed.

**Nothing is faked.** The package never tells Composer an extension exists and
never skips platform checks. Where a tool can't work on Windows, the package
says so and offers a real alternative instead of hiding the error.

## No Docker, WSL or Herd: `l4win.exe`

l4win comes in two parts:

- **`l4win.exe`** sets up the machine. It downloads the official PHP, Composer
  and Node.js builds into `%LOCALAPPDATA%\l4win` and puts `php`, `composer`,
  `node`, `npm` and `npx` on your PATH. No admin rights, nothing running in the
  background.
- **The Composer package** (the rest of this README) fixes the Laravel tools
  that don't work on Windows.

Each part works without the other. **Already have a Laravel app and a working
PHP** (Herd, Laragon, XAMPP, or PHP from php.net)? Install only the package
([Install](#install)) and `composer run dev`, Pail, Octane with FrankenPHP,
`vendor\bin\l4win sail`, `php artisan l4win:queues` and the doctor work on the
PHP you already use (`php artisan l4win` lists them).
`l4win.exe` adds the PHP, databases, Redis, mail and HTTPS sites, for when you
don't want those other tools.

```powershell
l4win setup                 # PHP, Composer, Node.js, and the commands on PATH
composer create-project laravel/laravel myapp
cd myapp
composer run dev
```

Each project gets the PHP version its `composer.json` asks for (or its
`.php-version`); a missing version is installed the first time it's needed.
`l4win list`, `l4win use php@8.4` and `l4win which` show and change versions.
Programs run inside a Windows Job Object, so stopping or killing a command also
stops everything it started. A killed `artisan serve` doesn't leave a PHP
server running.

Databases and mail come from the project's `.env` too:

```powershell
l4win up        # DB_CONNECTION=mysql → MySQL 8.4, mariadb → MariaDB (LTS),
                # pgsql → PostgreSQL 18 (superuser postgres, no password),
                # a redis cache/queue/session driver → Redis (see below),
                # MAIL_MAILER=smtp → Mailpit (inbox at http://localhost:8025);
                # creates DB_DATABASE and DB_USERNAME if they don't exist
l4win status    # what's running, on which port, where the data is
l4win down      # stop them
```

Services are shared by all projects (one MySQL, a database per project), run
hidden in the background until `l4win down`, and use the ports in `.env`
(`DB_PORT`, `MAIL_PORT`). If another program already has the port, such as a
MySQL installed as a Windows service, l4win names it and doesn't start. Data
lives in `%LOCALAPPDATA%\l4win\data`. `l4win db create NAME --user=… --password=…`
creates more databases on whichever database server is running.

**Redis:** Redis has no official Windows build, so l4win runs
[Garnet](https://github.com/microsoft/garnet), Microsoft's open-source server
that speaks the Redis protocol. Laravel's cache, locks, sessions and queues
(including delayed jobs, which use Lua scripts) work against it unchanged.
Garnet needs .NET; l4win downloads Microsoft's .NET runtime into its own folder
for Garnet only, without installing anything system-wide. Data is saved on
`l4win down` and loaded on the next start. Every PHP l4win installs also gets the
`redis` extension (`REDIS_CLIENT=phpredis`, Laravel's default); for PHP
versions installed earlier, run `l4win install phpredis`.

### Sites at `https://NAME.localhost`

```powershell
cd C:\code; l4win park      # every Laravel app in C:\code: https://FOLDER.localhost
cd C:\elsewhere\shop; l4win link     # or one project: https://shop.localhost
l4win up web                # start serving (l4win up in a site starts it too)
l4win trust                 # once: browsers trust the local certificates
l4win sites                 # list them; l4win open opens this project's site
```

Browsers send `*.localhost` to this machine by themselves, so there's no
hosts file to edit. Caddy (official build) serves the sites over HTTPS with a
certificate authority it creates on this machine; `l4win trust` adds that to
your user's trusted certificates (Windows asks to confirm, no admin needed).
PHP runs as `php-cgi.exe` workers, which come with every PHP for Windows, and
each site gets the PHP version its project asks for. A hidden l4win process
keeps the workers running and stops everything on `l4win down`. Set `APP_URL`
to the site's address. Command-line tools (curl, PHP's own HTTP client) don't
resolve `*.localhost` the way browsers do. Calls from one site to another need
`127.0.0.1` with a `Host` header.

The only thing it may ask for is Microsoft's Visual C++ runtime, which PHP for
Windows needs and most machines already have (`l4win install vcredist`).

**Another PHP or Composer still runs?** Windows searches the system PATH before
the user PATH, so a `php` or `composer` installed for all users (for example
Composer-Setup's `C:\ProgramData\ComposerSetup\bin`, or a PHP folder added by
XAMPP) wins over l4win's. `l4win setup` lists them. Uninstall that program, or
remove its folder from the system PATH (System Properties > Environment
Variables, needs admin), then open a new terminal; `where.exe composer` shows
which one runs. Until then, `l4win php` and `l4win composer` always use
l4win's. Folders on the user PATH (Herd's, for example) come after l4win's
and don't get in the way.

Download `l4win.exe` from the releases page, or from a project that has this
package, run `vendor\bin\l4win install-cli`: it downloads the release (checked
against its SHA-256) and runs `l4win setup`. To uninstall it, run
`l4win setup --undo` and delete `%LOCALAPPDATA%\l4win`. Building it is covered in
[cli/README.md](cli/README.md).

### Seeing every command

Type `l4win` in any terminal for the grouped list of commands. Inside a project
with this package, the list also includes the package's commands (`doctor`,
`queues`, `sail`), and `l4win help COMMAND` explains any of them. Without
`l4win.exe`, `vendor\bin\l4win` or `php artisan l4win` show the package's part of
the list and how to get the rest.

## What it covers

| Tool | On Windows with this package | How |
| --- | --- | --- |
| `php artisan pail` / `composer run dev` | Works | Pail's `tail -F` subprocess and pcntl signal traps replaced with pure PHP; Pail's capture, filters and output kept. |
| Octane + FrankenPHP | Works | Downloads FrankenPHP's official Windows build, and handles Ctrl+C without pcntl. |
| Octane + Swoole / RoadRunner | Clear error | Swoole has no Windows build. Use FrankenPHP. |
| Sail | Works | `vendor\bin\l4win sail`: Sail's commands without bash, using Docker Desktop. |
| Horizon | Runs inside Sail | Horizon needs pcntl + posix, so it runs in Sail's Linux containers. `php artisan l4win:queues` runs the same supervisors natively with Laravel's own workers. |
| Setup problems | Diagnosed | `php artisan l4win:doctor` / `vendor\bin\l4win doctor`. |

## Install

Until it's on Packagist, add the repository to the project's `composer.json`:

```json
"repositories": [
    { "type": "vcs", "url": "https://github.com/webteractive/l4win" }
]
```

```bash
composer require --dev webteractive/l4win:^0.1 -W
```

`-W` lets Composer upgrade an old Pail (1.0/1.1 required `ext-pcntl`). Laravel
auto-discovers the service provider; there is nothing to publish.

## Pail and `composer run dev`

```bash
composer run dev     # server, queue, logs, vite
php artisan pail     # --filter --message --level --user --timeout all work
```

Pail's service provider writes every log message (from any channel) into
`storage/pail/*.pail` while a session is open. The Windows command reads that
file with a pure-PHP tailer, uses Pail's own filters and printer, stops on
Ctrl+C via `sapi_windows_set_ctrl_handler()`, keeps its file fresh during quiet
sessions, and removes files left behind by force-killed sessions.

## Octane (FrankenPHP)

```bash
php artisan octane:install --server=frankenphp
php artisan octane:start          # with OCTANE_SERVER=frankenphp in .env
```

What changes on Windows:

- **Binary:** Octane only knows the Linux and macOS FrankenPHP downloads. The
  package downloads `frankenphp-windows-x86_64.zip` from FrankenPHP's latest
  release into `.frankenphp\` in the project (git-ignored). A `frankenphp.exe`
  already on your PATH is used first.
- **Signals:** Octane subscribes to `SIGINT`/`SIGTERM`/`SIGHUP`, which don't
  exist without pcntl. The package subscribes to none and stops the server on
  Ctrl+C through the Windows console handler. Stopping, reloading and status go
  through FrankenPHP's admin API, which already works on Windows.

Notes: the Windows archive bundles the official PHP for Windows, so enable the
extensions your app needs in the `php.ini` inside `.frankenphp\`. Windows builds
are x64 only. To refresh an outdated binary, delete `.frankenphp\` and start again.

## Sail

```powershell
vendor\bin\l4win sail up -d
vendor\bin\l4win sail artisan migrate
vendor\bin\l4win sail composer require laravel/sanctum
vendor\bin\l4win sail shell
```

Same commands and behaviour as `vendor/bin/sail` (it builds the same
`docker compose` calls), with these Windows details:

- Needs Docker Desktop. No bash or WSL shell is required to run it.
- `WWWUSER`/`WWWGROUP` default to `1000`, which Sail's images expect.
- `SAIL_FILES` takes `;` between paths on Windows (`C:\a.yml;C:\b.yml`).
- `l4win sail open` uses the Windows `start` command.

Keep shell scripts in LF: published Sail files under `docker/` that Git checked
out with CRLF fail inside the container (`bash\r: No such file or directory`).
The doctor detects this. Add `* text=auto eol=lf` to `.gitattributes`.

For large projects, Docker Desktop file sharing from a Windows path is slower
than from the WSL filesystem. That's a Docker Desktop property, not a Sail one.

## Horizon

Horizon requires `ext-pcntl` and `ext-posix` in its `composer.json` and uses
Unix process control to manage its workers, so it cannot run natively on
Windows, and this package does not pretend otherwise.

- **Projects that require Horizon:** run them with `l4win sail`. Horizon then
  runs inside Sail's Linux container, exactly as in production.
- **Running the queues natively:** `php artisan l4win:queues` reads
  `config/horizon.php` the way Horizon does (defaults merged per environment,
  wildcard environment names, supervisors with `maxProcesses` 0 skipped) and
  runs each supervisor with Laravel's `queue:listen`: same connection, queues,
  process count, tries, timeout, memory, sleep and backoff. Crashed workers are
  restarted with backoff. There's no dashboard, metrics or auto-balancing.

```bash
php artisan l4win:queues                 # current APP_ENV
php artisan l4win:queues --environment=production --max-processes=2
php artisan l4win:queues --dry-run       # print the worker commands
php artisan l4win:queues --work          # queue:work: faster, but timeouts
                                           # aren't enforced without pcntl
```

`queue:listen` is the default because it enforces job timeouts on Windows.

## Doctor

```bash
php artisan l4win:doctor
vendor\bin\l4win doctor     # works even if the app can't boot
```

Checks PHP version, `php.ini`, required and recommended extensions,
`memory_limit`, composer/node/npm/git, Git long paths and `core.autocrlf`,
`.env`/`APP_KEY`, writable folders, OneDrive locations, old Pail versions,
Octane's server choice, projects that require Horizon, packages that truly need
pcntl, and CRLF shell scripts in Sail's Docker files.

## Settings (environment variables)

| Variable | Default | Effect |
| --- | --- | --- |
| `L4WIN_PAIL` | `true` | `false` leaves Pail untouched. |
| `L4WIN_OCTANE` | `true` | `false` leaves Octane untouched. |
| `L4WIN_FORCE` | `false` | Apply the replacements on any OS (for testing). |

## Safety

- Replacements activate only on Windows or when pcntl is missing, and only when
  the package they replace is installed. Elsewhere the stock commands run.
- It's a dev dependency, so production installs (`--no-dev`) don't include it.
- No `provide` of platform extensions and no ignored platform requirements.

## Tests

```bash
php tests/smoke.php    # tailer, .env parsing, Sail commands, Horizon plan, doctor
php tests/compat.php   # subclasses load against Pail/Octane signature stubs
```

These run without Laravel. Verify on a real Windows machine before release:
`composer run dev`, `octane:start` with FrankenPHP, `l4win sail up`, and
`l4win:queues` against a Redis instance.
