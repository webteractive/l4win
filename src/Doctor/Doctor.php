<?php

namespace Webteractive\L4win\Doctor;

use Composer\InstalledVersions;
use Webteractive\L4win\Platform;

/**
 * Framework-free environment checks, so they also run from vendor/bin when
 * the Laravel app itself can't boot yet (e.g. a missing PHP extension).
 */
final class Doctor
{
    /** Extensions Laravel needs at runtime. */
    public const REQUIRED_EXTENSIONS = [
        'ctype', 'curl', 'dom', 'fileinfo', 'filter', 'hash', 'mbstring',
        'openssl', 'pcre', 'pdo', 'session', 'tokenizer', 'xml',
    ];

    /** Not strictly required, but their absence causes common Windows pain. */
    public const RECOMMENDED_EXTENSIONS = [
        'zip' => 'Composer falls back to slow, error-prone unzipping without it.',
        'intl' => 'Needed by Number/locale helpers and many packages.',
        'gd' => 'Needed for image handling (thumbnails, PDFs with images).',
    ];

    /** Packages that genuinely need pcntl/posix and have no Windows replacement here. */
    public const PCNTL_ONLY_PACKAGES = [
        'spatie/fork' => 'spatie/fork forks processes with pcntl and cannot run on Windows.',
    ];

    public function __construct(private readonly ?string $basePath = null)
    {
        //
    }

    /**
     * @return list<Check>
     */
    public function run(): array
    {
        return array_merge(
            $this->phpChecks(),
            $this->extensionChecks(),
            $this->toolChecks(),
            $this->projectChecks(),
            $this->packageChecks(),
            $this->lineEndingChecks(),
        );
    }

    /** @return list<Check> */
    private function phpChecks(): array
    {
        $checks = [];

        $checks[] = version_compare(PHP_VERSION, '8.2.0', '>=')
            ? Check::ok('PHP version', PHP_VERSION)
            : Check::fail('PHP version', PHP_VERSION, 'Laravel 11+ needs PHP 8.2 or newer.');

        $ini = php_ini_loaded_file();
        $checks[] = $ini
            ? Check::ok('php.ini', $ini)
            : Check::fail('php.ini', 'none loaded', 'Copy php.ini-development to php.ini next to php.exe, then enable extensions in it.');

        $memory = ini_get('memory_limit');
        $bytes = $this->toBytes((string) $memory);
        $checks[] = ($bytes === -1 || $bytes >= 512 * 1024 * 1024)
            ? Check::ok('memory_limit', (string) $memory)
            : Check::warn('memory_limit', (string) $memory, 'Set memory_limit = 512M (or -1) in php.ini; Composer and tests can run out.');

        $checks[] = Platform::hasPcntl()
            ? Check::ok('pcntl', 'available')
            : Check::info('pcntl', 'not available (normal on Windows)', 'Pail runs in Windows mode through this package.');

        return $checks;
    }

    /** @return list<Check> */
    private function extensionChecks(): array
    {
        $missing = array_values(array_filter(self::REQUIRED_EXTENSIONS, fn (string $ext) => ! extension_loaded($ext)));

        $checks = [];

        if ($missing === []) {
            $checks[] = Check::ok('Required extensions', 'all loaded');
        } else {
            $hint = 'Enable in php.ini: '.implode(', ', array_map(fn ($e) => "extension={$e}", $missing));

            if (count($missing) > 3) {
                $hint .= '. Many missing usually means extension_dir is wrong; set extension_dir = "ext".';
            }

            $checks[] = Check::fail('Required extensions', 'missing: '.implode(', ', $missing), $hint);
        }

        foreach (self::RECOMMENDED_EXTENSIONS as $ext => $why) {
            if (! extension_loaded($ext)) {
                $checks[] = Check::warn("ext-{$ext}", 'not loaded', "{$why} Add extension={$ext} to php.ini.");
            }
        }

        return $checks;
    }

    /** @return list<Check> */
    private function toolChecks(): array
    {
        $checks = [];

        foreach (['composer' => true, 'node' => true, 'npm' => true, 'git' => false] as $tool => $important) {
            $path = Platform::which($tool);

            if ($path !== null) {
                $checks[] = Check::ok($tool, $path);
            } elseif ($important) {
                $checks[] = Check::warn($tool, 'not on PATH', "`composer run dev` needs {$tool}. Install it and reopen the terminal.");
            } else {
                $checks[] = Check::info($tool, 'not on PATH');
            }
        }

        if (Platform::isWindows() && Platform::which('git') !== null) {
            $longPaths = trim((string) @shell_exec('git config --get core.longpaths 2>NUL'));

            if (strtolower($longPaths) !== 'true') {
                $checks[] = Check::warn('git core.longpaths', $longPaths === '' ? 'unset' : $longPaths, 'Run `git config --global core.longpaths true` to avoid path-too-long errors in vendor/ and node_modules/.');
            }
        }

        return $checks;
    }

    /** @return list<Check> */
    private function projectChecks(): array
    {
        if ($this->basePath === null || ! is_file($this->basePath.'/artisan')) {
            return [];
        }

        $base = $this->basePath;
        $checks = [];

        $env = $base.'/.env';

        if (! is_file($env)) {
            $checks[] = Check::fail('.env', 'missing', 'Run `copy .env.example .env` then `php artisan key:generate`.');
        } elseif (! preg_match('/^APP_KEY=\S+/m', (string) file_get_contents($env))) {
            $checks[] = Check::fail('APP_KEY', 'empty', 'Run `php artisan key:generate`.');
        } else {
            $checks[] = Check::ok('.env / APP_KEY', 'present');
        }

        foreach (['storage', 'bootstrap/cache'] as $dir) {
            $path = $base.'/'.$dir;
            $checks[] = is_dir($path) && is_writable($path)
                ? Check::ok("{$dir} writable", 'yes')
                : Check::fail("{$dir} writable", 'no', "Make sure {$dir} exists and isn't read-only or locked by antivirus/OneDrive.");
        }

        if (! file_exists($base.'/public/storage')) {
            $checks[] = Check::info('public/storage link', 'missing', 'Run `php artisan storage:link` if the app serves uploaded files.');
        }

        if (Platform::isWindows() && preg_match('/onedrive/i', $base)) {
            $checks[] = Check::warn('Project location', $base, 'Projects inside OneDrive suffer file locks and slow syncs. Move it to e.g. C:\\projects.');
        }

        return $checks;
    }

    /** @return list<Check> */
    private function packageChecks(): array
    {
        $checks = [];
        $required = $this->composerRequires();
        $installed = fn (string $package): bool => class_exists(InstalledVersions::class) && InstalledVersions::isInstalled($package);

        if ($installed('laravel/pail')) {
            $version = ltrim((string) InstalledVersions::getPrettyVersion('laravel/pail'), 'v');

            $checks[] = preg_match('/^\d/', $version) === 1 && version_compare($version, '1.2.0', '<')
                ? Check::fail('laravel/pail', $version, 'Pail before 1.2 requires ext-pcntl to install. Run `composer update laravel/pail`.')
                : Check::ok('laravel/pail', Platform::needsShims() ? 'Windows mode active' : 'native');
        }

        if (! Platform::isWindows()) {
            return $checks;
        }

        if (isset($required['laravel/horizon'])) {
            $checks[] = Check::warn(
                'laravel/horizon',
                'required by this project',
                'Horizon needs ext-pcntl and ext-posix, so Composer cannot install this project natively on Windows. '
                .'Run it with `vendor\\bin\\l4win sail up` (Horizon runs inside Linux containers) or in WSL. '
                .'For local queue work without Horizon, `php artisan l4win:queues` runs the same supervisors.'
            );
        }

        if ($installed('laravel/octane') || isset($required['laravel/octane'])) {
            $server = $this->envValue('OCTANE_SERVER') ?? 'roadrunner';

            $checks[] = $server === 'frankenphp'
                ? Check::ok('laravel/octane', 'FrankenPHP (Windows build supported)')
                : Check::warn('laravel/octane', "server: {$server}", 'Only FrankenPHP runs natively on Windows. Set OCTANE_SERVER=frankenphp in .env.');
        }

        if ($installed('laravel/sail') || isset($required['laravel/sail'])) {
            $checks[] = Check::info('laravel/sail', 'available', 'Use `vendor\\bin\\l4win sail` instead of `vendor/bin/sail` (no bash needed, Docker Desktop required).');
        }

        foreach (self::PCNTL_ONLY_PACKAGES as $package => $hint) {
            if ($installed($package) || isset($required[$package])) {
                $checks[] = Check::fail($package, 'required', $hint);
            }
        }

        return $checks;
    }

    /**
     * Shell scripts with Windows line endings break inside Linux containers
     * ("/usr/bin/env: 'bash\r': No such file or directory"), the most common
     * Sail failure on Windows.
     *
     * @return list<Check>
     */
    private function lineEndingChecks(): array
    {
        if ($this->basePath === null || ! Platform::isWindows()) {
            return [];
        }

        $checks = [];

        if (Platform::which('git') !== null) {
            $autocrlf = strtolower(trim((string) @shell_exec('git -C '.escapeshellarg($this->basePath).' config --get core.autocrlf 2>NUL')));

            if ($autocrlf === 'true') {
                $checks[] = Check::warn('git core.autocrlf', 'true', 'Git converts files to CRLF on checkout, which breaks shell scripts in Docker. Add `* text=auto eol=lf` to .gitattributes or run `git config core.autocrlf input`.');
            }
        }

        $broken = [];

        foreach (['docker', 'vendor/laravel/sail/runtimes'] as $dir) {
            $path = $this->basePath.DIRECTORY_SEPARATOR.$dir;

            if (! is_dir($path)) {
                continue;
            }

            $files = new \RecursiveIteratorIterator(new \RecursiveDirectoryIterator($path, \FilesystemIterator::SKIP_DOTS));

            foreach ($files as $file) {
                if (! $file->isFile() || $file->getSize() > 1024 * 1024) {
                    continue;
                }

                $head = (string) @file_get_contents($file->getPathname(), false, null, 0, 4096);

                if (str_starts_with($head, '#!') && str_contains($head, "\r\n")) {
                    $broken[] = substr($file->getPathname(), strlen($this->basePath) + 1);
                }
            }
        }

        if ($broken !== []) {
            $checks[] = Check::fail(
                'Script line endings',
                count($broken).' script(s) with CRLF',
                'Convert to LF before building Sail images: '.implode(', ', array_slice($broken, 0, 5)).(count($broken) > 5 ? ', …' : '')
            );
        }

        return $checks;
    }

    /**
     * @return array<string, string>
     */
    private function composerRequires(): array
    {
        if ($this->basePath === null || ! is_file($file = $this->basePath.'/composer.json')) {
            return [];
        }

        $json = json_decode((string) file_get_contents($file), true);

        return is_array($json) ? array_merge((array) ($json['require'] ?? []), (array) ($json['require-dev'] ?? [])) : [];
    }

    private function envValue(string $key): ?string
    {
        if (($value = getenv($key)) !== false && $value !== '') {
            return $value;
        }

        if ($this->basePath === null || ! is_file($file = $this->basePath.'/.env')) {
            return null;
        }

        $values = \Webteractive\L4win\Sail\DotEnv::parse((string) file_get_contents($file));

        return ($values[$key] ?? '') !== '' ? $values[$key] : null;
    }

    private function toBytes(string $value): int
    {
        $value = trim($value);

        if ($value === '-1') {
            return -1;
        }

        $number = (int) $value;

        return match (strtolower(substr($value, -1))) {
            'g' => $number * 1024 ** 3,
            'm' => $number * 1024 ** 2,
            'k' => $number * 1024,
            default => $number,
        };
    }
}
