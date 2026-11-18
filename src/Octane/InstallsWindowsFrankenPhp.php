<?php

namespace Webteractive\L4win\Octane;

use Illuminate\Support\Facades\Http;
use RuntimeException;
use Symfony\Component\Process\Process;
use Webteractive\L4win\Platform;
use ZipArchive;

use function Laravel\Prompts\confirm;

/**
 * Octane only knows the Linux and macOS FrankenPHP binaries. Since FrankenPHP
 * 1.12 there is an official Windows build: a zip with frankenphp.exe plus the
 * official PHP for Windows it links against. We install it into .frankenphp/
 * inside the project (git-ignored), next to where Octane would put its binary.
 */
trait InstallsWindowsFrankenPhp
{
    protected function ensureFrankenPhpBinaryIsInstalled()
    {
        if (! Platform::isWindows()) {
            return parent::ensureFrankenPhpBinaryIsInstalled();
        }

        if (! is_null($binary = $this->findFrankenPhpBinary())) {
            return $binary;
        }

        if (is_file($local = $this->windowsFrankenPhpPath())) {
            return $local;
        }

        if (confirm('Unable to locate FrankenPHP. Download the official Windows build into .frankenphp/?', true)) {
            return $this->downloadFrankenPhpBinary();
        }

        return $local;
    }

    protected function downloadFrankenPhpBinary()
    {
        if (! Platform::isWindows()) {
            return parent::downloadFrankenPhpBinary();
        }

        $arch = strtolower(php_uname('m'));

        if (! in_array($arch, ['amd64', 'x86_64', 'x64'], true)) {
            throw new RuntimeException("FrankenPHP publishes Windows builds for x64 only (this machine reports [{$arch}]).");
        }

        $url = env('OCTANE_GITHUB_PROXY_PREFIX', '')
            .'https://github.com/php/frankenphp/releases/latest/download/frankenphp-windows-x86_64.zip';

        $directory = base_path('.frankenphp');
        $zip = sys_get_temp_dir().DIRECTORY_SEPARATOR.'frankenphp-windows-'.getmypid().'.zip';

        $this->components->info('Downloading FrankenPHP for Windows…');

        try {
            Http::withOptions(['sink' => $zip])->timeout(600)->get($url)->throw();

            $this->extractZip($zip, $directory);
        } finally {
            @unlink($zip);
        }

        if (! is_file($directory.DIRECTORY_SEPARATOR.'.gitignore')) {
            file_put_contents($directory.DIRECTORY_SEPARATOR.'.gitignore', "*\n");
        }

        $binary = $this->locateExtractedBinary($directory);

        if ($binary === null) {
            throw new RuntimeException('Downloaded the FrankenPHP archive, but it did not contain frankenphp.exe.');
        }

        return $binary;
    }

    protected function windowsFrankenPhpPath(): string
    {
        return $this->locateExtractedBinary(base_path('.frankenphp'))
            ?? base_path('.frankenphp'.DIRECTORY_SEPARATOR.'frankenphp.exe');
    }

    protected function extractZip(string $zip, string $directory): void
    {
        if (! is_dir($directory)) {
            mkdir($directory, 0755, true);
        }

        if (class_exists(ZipArchive::class)) {
            $archive = new ZipArchive;

            if ($archive->open($zip) !== true || ! $archive->extractTo($directory)) {
                throw new RuntimeException('Unable to extract the FrankenPHP archive.');
            }

            $archive->close();

            return;
        }

        // ext-zip is optional; PowerShell ships with every supported Windows.
        $process = new Process([
            'powershell', '-NoProfile', '-NonInteractive', '-Command',
            'Expand-Archive -Force -LiteralPath $env:LW_ZIP -DestinationPath $env:LW_DIR',
        ], null, ['LW_ZIP' => $zip, 'LW_DIR' => $directory], null, 300);

        $process->mustRun();
    }

    protected function locateExtractedBinary(string $directory): ?string
    {
        if (! is_dir($directory)) {
            return null;
        }

        $candidates = array_merge(
            glob($directory.DIRECTORY_SEPARATOR.'frankenphp.exe') ?: [],
            glob($directory.DIRECTORY_SEPARATOR.'*'.DIRECTORY_SEPARATOR.'frankenphp.exe') ?: [],
        );

        return $candidates[0] ?? null;
    }
}
