<?php

namespace Webteractive\L4win\Console;

use Illuminate\Console\Command;
use Symfony\Component\Process\PhpExecutableFinder;
use Symfony\Component\Process\Process;
use Webteractive\L4win\Platform;
use Webteractive\L4win\Queues\HorizonPlan;

/**
 * Runs the supervisors from config/horizon.php as plain Laravel queue workers.
 *
 * This is not Horizon (no dashboard, metrics or auto-balancing). It processes
 * the same queues, on the same connection, with the same tries, timeouts and
 * process counts, and restarts workers that exit, which is what matters when
 * developing on Windows.
 */
class QueuesCommand extends Command
{
    protected $signature = 'l4win:queues
        {--environment= : The Horizon environment to use (defaults to the app environment)}
        {--max-processes= : Cap the number of processes per supervisor}
        {--work : Use queue:work (faster, but job timeouts are not enforced without pcntl)}
        {--dry-run : Show the worker commands without starting them}';

    protected $description = 'Run the queues defined in config/horizon.php using Windows-compatible workers';

    protected bool $stopRequested = false;

    /** @var array<int, array{label: string, command: list<string>, process: Process, color: string, restarts: int}> */
    protected array $workers = [];

    public function handle(): int
    {
        $horizon = config('horizon');

        if (! is_array($horizon) || empty($horizon['environments'])) {
            $this->components->error('No config/horizon.php found. Run your queues with `php artisan queue:listen` instead.');

            return self::FAILURE;
        }

        $environment = (string) ($this->option('environment') ?: $this->laravel->environment());
        $cap = $this->option('max-processes') !== null ? (int) $this->option('max-processes') : null;
        $useWork = (bool) $this->option('work');

        $supervisors = (new HorizonPlan($horizon, (array) config('queue.connections', [])))
            ->supervisors($environment, $cap);

        if ($supervisors === []) {
            $this->components->error("config/horizon.php has no supervisors for the [{$environment}] environment.");

            return self::FAILURE;
        }

        $php = (new PhpExecutableFinder)->find(false) ?: PHP_BINARY;
        $colors = ['cyan', 'magenta', 'green', 'yellow', 'blue', 'red'];

        foreach ($supervisors as $index => $supervisor) {
            $command = HorizonPlan::command($supervisor, $php, $useWork);

            for ($i = 1; $i <= $supervisor['processes']; $i++) {
                $this->workers[] = [
                    'label' => $supervisor['processes'] > 1 ? "{$supervisor['name']}#{$i}" : $supervisor['name'],
                    'command' => $command,
                    'process' => $this->makeProcess($command),
                    'color' => $colors[$index % count($colors)],
                    'restarts' => 0,
                ];
            }
        }

        $this->components->info(sprintf(
            'Running %d worker(s) for the [%s] Horizon environment using %s.',
            count($this->workers), $environment, $useWork ? 'queue:work' : 'queue:listen'
        ));

        foreach ($supervisors as $supervisor) {
            $this->line(sprintf(
                '  <fg=gray>%s</> %s on <fg=white>%s</> ×%d, tries=%d, timeout=%ds',
                $supervisor['name'], implode(',', $supervisor['queues']), $supervisor['connection'],
                $supervisor['processes'], $supervisor['tries'], $supervisor['timeout'],
            ));
        }

        if ($useWork && ! Platform::hasPcntl()) {
            $this->components->warn('queue:work cannot kill jobs that exceed their timeout without pcntl. Drop --work to have timeouts enforced.');
        }

        if ($this->option('dry-run')) {
            $this->newLine();

            foreach ($this->workers as $worker) {
                $this->line('  '.implode(' ', $worker['command']));
            }

            return self::SUCCESS;
        }

        $this->registerStopHandlers();
        $this->line('  <fg=gray>Press Ctrl+C to stop.</>');
        $this->newLine();

        foreach ($this->workers as $worker) {
            $worker['process']->start();
        }

        while (! $this->stopRequested) {
            foreach ($this->workers as $key => $worker) {
                $this->flushOutput($worker);

                if (! $worker['process']->isRunning() && ! $this->stopRequested) {
                    $this->restart($key);
                }
            }

            usleep(200_000);
        }

        $this->newLine();
        $this->components->info('Stopping workers…');

        foreach ($this->workers as $worker) {
            $worker['process']->stop(10);
            $this->flushOutput($worker);
        }

        return self::SUCCESS;
    }

    /**
     * @param  list<string>  $command
     */
    protected function makeProcess(array $command): Process
    {
        return new Process($command, $this->laravel->basePath(), null, null, null);
    }

    protected function restart(int $key): void
    {
        $worker = $this->workers[$key];
        $exitCode = $worker['process']->getExitCode();

        $this->line(sprintf('  <fg=%s>[%s]</> <fg=yellow>exited (code %s), restarting…</>', $worker['color'], $worker['label'], $exitCode ?? '?'));

        // Back off when a worker keeps crashing (bad config, Redis down), like Horizon does.
        if ($worker['restarts'] > 0) {
            sleep(min(30, 2 ** min($worker['restarts'], 5)));
        }

        $worker['process'] = $this->makeProcess($worker['command']);
        $worker['process']->start();
        $worker['restarts']++;

        $this->workers[$key] = $worker;
    }

    /**
     * @param  array{label: string, process: Process, color: string}  $worker
     */
    protected function flushOutput(array $worker): void
    {
        $output = $worker['process']->getIncrementalOutput().$worker['process']->getIncrementalErrorOutput();

        foreach (preg_split('/\r?\n/', rtrim($output)) ?: [] as $line) {
            if (trim($line) !== '') {
                $this->line(sprintf('  <fg=%s>[%s]</> %s', $worker['color'], $worker['label'], $line));
            }
        }
    }

    protected function registerStopHandlers(): void
    {
        if (function_exists('sapi_windows_set_ctrl_handler')) {
            @sapi_windows_set_ctrl_handler(function (): void {
                $this->stopRequested = true;
            });
        }

        if (Platform::hasPcntl()) {
            $this->trap([SIGINT, SIGTERM], function (): void {
                $this->stopRequested = true;
            });
        }
    }
}
