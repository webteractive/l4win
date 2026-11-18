# l4win

**Laravel for Windows.** Run Laravel projects natively on Windows. Install it once per project as a dev
dependency and the Unix-only parts of the Laravel toolchain get Windows
replacements. On WSL, Linux and macOS it does nothing, so the whole team can
keep it installed.

**Nothing is faked.** The package never tells Composer an extension exists and
never skips platform checks. Where a tool can't work on Windows, the package
says so and offers a real alternative instead of hiding the error.

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
