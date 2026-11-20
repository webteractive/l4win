<?php

namespace Webteractive\L4win\Cli;

use RuntimeException;

/**
 * Downloads the released l4win.exe (checked against its published SHA-256)
 * into %LOCALAPPDATA%\l4win\bin. Running its `setup` afterwards installs
 * PHP, Composer and Node.js and puts `l4win` on the PATH.
 */
final class CliInstaller
{
    public const REPOSITORY = 'webteractive/l4win';

    public const TAG_PREFIX = 'cli-v';

    /**
     * @param  callable(string): mixed  $fetch  url => body (string) or false; injectable for tests
     */
    public function __construct(private $fetch = null)
    {
        $this->fetch ??= self::httpGet(...);
    }

    public static function target(): string
    {
        $home = getenv('L4WIN_HOME');

        if ($home === false || $home === '') {
            $local = getenv('LOCALAPPDATA');

            if ($local === false || $local === '') {
                throw new RuntimeException('LOCALAPPDATA is not set; set L4WIN_HOME to the folder l4win should use.');
            }

            $home = $local.'\\l4win';
        }

        return $home.'\\bin\\l4win.exe';
    }

    /**
     * Picks the newest l4win.exe release from a GitHub releases list.
     *
     * @param  list<array<string, mixed>>  $releases
     * @return array{version: string, exe: string, sha256: string}
     */
    public static function pickRelease(array $releases): array
    {
        foreach ($releases as $release) {
            $tag = (string) ($release['tag_name'] ?? '');

            if (! str_starts_with($tag, self::TAG_PREFIX) || ! empty($release['draft']) || ! empty($release['prerelease'])) {
                continue;
            }

            $assets = [];

            foreach ($release['assets'] ?? [] as $asset) {
                $assets[$asset['name'] ?? ''] = (string) ($asset['browser_download_url'] ?? '');
            }

            if (isset($assets['l4win.exe'], $assets['l4win.exe.sha256'])) {
                return [
                    'version' => substr($tag, strlen(self::TAG_PREFIX)),
                    'exe' => $assets['l4win.exe'],
                    'sha256' => $assets['l4win.exe.sha256'],
                ];
            }
        }

        throw new RuntimeException('No l4win.exe release found at https://github.com/'.self::REPOSITORY.'/releases yet.');
    }

    /**
     * Downloads and verifies the newest release. Returns [version, path].
     *
     * @return array{0: string, 1: string}
     */
    public function install(string $target): array
    {
        $json = ($this->fetch)('https://api.github.com/repos/'.self::REPOSITORY.'/releases?per_page=30');
        $releases = is_string($json) ? json_decode($json, true) : null;

        if (! is_array($releases)) {
            throw new RuntimeException('Could not read the release list from GitHub.');
        }

        $release = self::pickRelease($releases);

        $sum = ($this->fetch)($release['sha256']);
        $expected = is_string($sum) ? strtolower((string) strtok(trim($sum), " \t*")) : '';

        if (! preg_match('/^[0-9a-f]{64}$/', $expected)) {
            throw new RuntimeException('The release has no valid l4win.exe.sha256.');
        }

        $exe = ($this->fetch)($release['exe']);

        if (! is_string($exe) || $exe === '') {
            throw new RuntimeException('Downloading l4win.exe failed.');
        }

        if (! hash_equals($expected, hash('sha256', $exe))) {
            throw new RuntimeException('l4win.exe does not match its published SHA-256; not installing it.');
        }

        $dir = dirname($target);

        if (! is_dir($dir) && ! mkdir($dir, 0755, true) && ! is_dir($dir)) {
            throw new RuntimeException("Could not create {$dir}.");
        }

        $tmp = $target.'.download';

        if (file_put_contents($tmp, $exe) === false) {
            throw new RuntimeException("Could not write {$tmp}.");
        }

        // A running l4win.exe can't be replaced, but it can be renamed.
        if (is_file($target) && ! @unlink($target)) {
            @rename($target, $target.'.old-'.getmypid());
        }

        if (! rename($tmp, $target)) {
            @unlink($tmp);
            throw new RuntimeException("Could not move l4win.exe into {$dir}.");
        }

        return [$release['version'], $target];
    }

    private static function httpGet(string $url): string|false
    {
        $headers = "User-Agent: l4win-package\r\n";

        if (str_starts_with($url, 'https://api.github.com/')) {
            $headers .= "Accept: application/vnd.github+json\r\n";
        }

        $context = stream_context_create(['http' => [
            'header' => $headers,
            'follow_location' => 1,
            'timeout' => 60,
        ]]);

        return @file_get_contents($url, false, $context);
    }
}
