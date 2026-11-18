<?php

namespace Webteractive\L4win\Queues;

/**
 * Reads config/horizon.php the same way Horizon's ProvisioningPlan does
 * (defaults merged into each environment, environment names matched with
 * wildcards, supervisors with maxProcesses < 1 skipped) and turns each
 * supervisor into plain `php artisan queue:*` worker commands.
 *
 * Horizon itself needs pcntl/posix, so this does not run Horizon. It runs the
 * same queues with the same options using Laravel's own workers.
 */
final class HorizonPlan
{
    /**
     * @param  array<string, mixed>  $horizon  config('horizon')
     * @param  array<string, mixed>  $queueConnections  config('queue.connections')
     */
    public function __construct(
        private readonly array $horizon,
        private readonly array $queueConnections = [],
    ) {
        //
    }

    /**
     * @return list<array{name: string, connection: string, queues: list<string>, processes: int, tries: int, timeout: int, memory: int, sleep: int|float, backoff: string, force: bool}>
     */
    public function supervisors(string $environment, ?int $maxProcessesCap = null): array
    {
        $plan = $this->environmentPlan($environment);
        $supervisors = [];

        foreach ($plan as $name => $options) {
            if (! is_array($options)) {
                continue;
            }

            $processes = (int) ($options['processes'] ?? $options['maxProcesses'] ?? 1);

            if ($processes < 1) {
                continue;
            }

            if ($maxProcessesCap !== null && $maxProcessesCap > 0) {
                $processes = min($processes, $maxProcessesCap);
            }

            $connection = (string) ($options['connection'] ?? 'redis');

            $supervisors[] = [
                'name' => (string) $name,
                'connection' => $connection,
                'queues' => $this->queues($options['queue'] ?? null, $connection),
                'processes' => $processes,
                'tries' => (int) ($options['tries'] ?? 0),
                'timeout' => (int) ($options['timeout'] ?? 60),
                'memory' => (int) ($options['memory'] ?? 128),
                'sleep' => $options['sleep'] ?? 3,
                'backoff' => is_array($options['backoff'] ?? null)
                    ? implode(',', $options['backoff'])
                    : (string) ($options['backoff'] ?? 0),
                'force' => (bool) ($options['force'] ?? false),
            ];
        }

        return $supervisors;
    }

    /**
     * @param  array{name: string, connection: string, queues: list<string>, tries: int, timeout: int, memory: int, sleep: int|float, backoff: string, force: bool}  $supervisor
     * @return list<string>
     */
    public static function command(array $supervisor, string $php, bool $useWork): array
    {
        $command = [
            $php,
            'artisan',
            $useWork ? 'queue:work' : 'queue:listen',
            $supervisor['connection'],
            '--name='.$supervisor['name'],
            '--queue='.implode(',', $supervisor['queues']),
            '--tries='.$supervisor['tries'],
            '--timeout='.$supervisor['timeout'],
            '--memory='.$supervisor['memory'],
            '--sleep='.$supervisor['sleep'],
            '--backoff='.$supervisor['backoff'],
        ];

        if ($supervisor['force']) {
            $command[] = '--force';
        }

        return $command;
    }

    /**
     * @return array<string, mixed>
     */
    private function environmentPlan(string $environment): array
    {
        $defaults = is_array($this->horizon['defaults'] ?? null) ? $this->horizon['defaults'] : [];
        $environments = is_array($this->horizon['environments'] ?? null) ? $this->horizon['environments'] : [];

        foreach ($environments as $pattern => $supervisors) {
            if (is_array($supervisors) && self::matches((string) $pattern, $environment)) {
                return array_replace_recursive($defaults, $supervisors);
            }
        }

        return [];
    }

    /**
     * @return list<string>
     */
    private function queues(mixed $queue, string $connection): array
    {
        $queues = is_array($queue) ? $queue : explode(',', (string) $queue);
        $queues = array_values(array_filter(array_map('trim', array_map('strval', $queues)), fn ($q) => $q !== ''));

        if ($queues === []) {
            $queues = [(string) ($this->queueConnections[$connection]['queue'] ?? 'default')];
        }

        return $queues;
    }

    /**
     * Same semantics as Str::is(): `*` matches anything.
     */
    private static function matches(string $pattern, string $value): bool
    {
        if ($pattern === $value) {
            return true;
        }

        $regex = '#^'.str_replace('\*', '.*', preg_quote($pattern, '#')).'\z#u';

        return preg_match($regex, $value) === 1;
    }
}
