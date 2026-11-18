<?php

namespace Webteractive\L4win\Console;

use Laravel\Pail\Console\Commands\PailCommand;
use Laravel\Pail\File;
use Laravel\Pail\Options;
use Laravel\Pail\Printers\CliPrinter;
use Laravel\Pail\ProcessFactory;
use Laravel\Pail\ValueObjects\MessageLogged;
use Throwable;
use Webteractive\L4win\Platform;
use Webteractive\L4win\Support\FileTailer;

use function Termwind\render;
use function Termwind\renderUsing;

/**
 * Drop-in replacement for `php artisan pail` on systems without pcntl.
 *
 * Stock Pail needs pcntl (for signal traps) and shells out to `tail -F`, and
 * neither exists on native Windows. This version keeps Pail's own log capture
 * (its service provider still writes every log message into storage/pail/*.pail),
 * its filters and its printer, and replaces only the tail loop and the signal
 * handling with pure PHP.
 */
class WindowsPailCommand extends PailCommand
{
    /** Pail deletes .pail files untouched for an hour; refresh ours well before that. */
    protected const KEEP_ALIVE_SECONDS = 300;

    protected const POLL_MICROSECONDS = 150_000;

    protected bool $stopRequested = false;

    /**
     * Same signature as the parent so Laravel can call it the same way;
     * the process factory is unused because we don't spawn `tail`.
     */
    public function handle(ProcessFactory $processFactory): void
    {
        $this->renderHeader();
        $this->removeOrphanedFiles();

        $this->file = new File(storage_path('pail/win-'.getmypid().'-'.uniqid().'.pail'));
        $this->file->create();
        $this->registerShutdownHandlers();

        $options = Options::fromCommand($this);
        $printer = new CliPrinter($this->output, $this->laravel->basePath());
        $tailer = new FileTailer((string) $this->file);

        $deadline = $options->timeout() > 0 ? microtime(true) + $options->timeout() : null;
        $lastKeepAlive = time();

        try {
            while (! $this->stopRequested) {
                foreach ($tailer->read() as $line) {
                    $this->printLine($line, $options, $printer);
                }

                if ($deadline !== null && microtime(true) >= $deadline) {
                    $this->components->info('Maximum execution time exceeded.');
                    break;
                }

                if (time() - $lastKeepAlive >= static::KEEP_ALIVE_SECONDS) {
                    $this->keepAlive();
                    $lastKeepAlive = time();
                }

                usleep(static::POLL_MICROSECONDS);
            }

            if ($this->stopRequested) {
                $this->newLine();
            }
        } finally {
            $this->cleanup();
        }
    }

    public function __destruct()
    {
        $this->cleanup();
    }

    protected function printLine(string $line, Options $options, CliPrinter $printer): void
    {
        try {
            $message = MessageLogged::fromJson($line);
        } catch (Throwable) {
            $this->output->writeln('  <fg=yellow>⚠ Pail skipped a malformed log line.</>');

            return;
        }

        if ($options->accepts($message)) {
            $printer->print($message);
        }
    }

    protected function renderHeader(): void
    {
        renderUsing($this->output);

        render(<<<'HTML'
            <div class="max-w-150 mx-2 mt-1 flex">
                <div>
                    <span class="px-1 bg-blue uppercase text-white">INFO</span>
                    <span class="flex-1">
                        <span class="ml-1">Tailing application logs.</span>
                    </span>
                </div>
                <span class="flex-1"></span>
                <span class="text-gray ml-1">Press Ctrl+C to exit</span>
            </div>
            HTML);

        render(<<<'HTML'
            <div class="max-w-150 mx-2 flex">
                <span class="text-gray">Windows mode (l4win)</span>
                <span class="flex-1"></span>
                <span class="text-gray ml-1">Use -v|-vv to show more details</span>
            </div>
            HTML);
    }

    /**
     * Ctrl+C handling without pcntl. On Windows, PHP exposes console control
     * events through sapi_windows_set_ctrl_handler().
     */
    protected function registerShutdownHandlers(): void
    {
        if (function_exists('sapi_windows_set_ctrl_handler')) {
            @sapi_windows_set_ctrl_handler(function (): void {
                $this->stopRequested = true;
            });
        }

        register_shutdown_function(fn () => $this->cleanup());
    }

    /**
     * Keep our file from being treated as stale during long, quiet sessions,
     * and recreate it if something removed it.
     */
    protected function keepAlive(): void
    {
        if ($this->file === null) {
            return;
        }

        $this->file->create();
        @touch((string) $this->file);
    }

    /**
     * Tools like `concurrently --kill-others` force-kill processes on Windows,
     * so a previous session may have left its .pail file behind. Pail would keep
     * writing to it, so remove files whose owning process is gone.
     */
    protected function removeOrphanedFiles(): void
    {
        foreach (glob(storage_path('pail/win-*-*.pail')) ?: [] as $path) {
            if (preg_match('/win-(\d+)-[^\\\\\/]+\.pail$/', $path, $match) !== 1) {
                continue;
            }

            if (! Platform::isProcessAlive((int) $match[1])) {
                @unlink($path);
            }
        }
    }

    protected function cleanup(): void
    {
        if ($this->file === null) {
            return;
        }

        $path = (string) $this->file;
        $this->file = null;

        // On Windows another process may be mid-write; a failed unlink is
        // harmless because the orphan sweep removes it next time.
        if (is_file($path)) {
            @unlink($path);
        }
    }
}
