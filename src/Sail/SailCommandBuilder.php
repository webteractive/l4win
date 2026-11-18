<?php

namespace Webteractive\L4win\Sail;

/**
 * Builds the same Docker commands as Laravel Sail's bash script
 * (vendor/laravel/sail/bin/sail), without needing bash.
 *
 * Pure: takes the environment and state, returns what to run. The launcher in
 * bin/l4win sail does the I/O.
 */
final class SailCommandBuilder
{
    /** AI agent variables Sail forwards into the container. */
    public const AGENT_ENV_VARS = [
        'AI_AGENT', 'CLAUDECODE', 'CLAUDE_CODE', 'CLAUDE_CODE_IS_COWORK', 'CURSOR_TRACE_ID',
        'CURSOR_AGENT', 'GEMINI_CLI', 'CODEX_SANDBOX', 'CODEX_CI', 'CODEX_THREAD_ID',
        'AUGMENT_AGENT', 'OPENCODE_CLIENT', 'OPENCODE', 'AMP_CURRENT_THREAD_ID', 'REPL_ID',
        'ANTIGRAVITY_AGENT', 'COPILOT_MODEL', 'COPILOT_ALLOW_ALL', 'COPILOT_GITHUB_TOKEN',
        'COPILOT_CLI', 'PI_CODING_AGENT', 'KIRO_AGENT_PATH',
    ];

    /** Sub-commands that run inside the app container as the Sail user. */
    private const APP_COMMANDS = [
        'php' => ['php'],
        'composer' => ['composer'],
        'cpx' => ['cpx'],
        'test' => ['php', 'artisan', 'test'],
        'phpunit' => ['php', 'vendor/bin/phpunit'],
        'pest' => ['php', 'vendor/bin/pest'],
        'pint' => ['php', 'vendor/bin/pint'],
        'tinker' => ['php', 'artisan', 'tinker'],
        'node' => ['node'],
        'npm' => ['npm'],
        'npx' => ['npx'],
        'pnpm' => ['pnpm'],
        'pnpx' => ['pnpx'],
        'yarn' => ['yarn'],
        'bun' => ['bun'],
        'bunx' => ['bunx'],
        'shell' => ['bash'],
        'bash' => ['bash'],
    ];

    /**
     * @param  array<string, string>  $env  Resolved environment (see resolveEnvironment()).
     * @param  list<string>  $compose  e.g. ['docker', 'compose'] plus any -f files.
     */
    public function __construct(
        private readonly array $env,
        private readonly array $compose,
        private readonly bool $stdinIsTty = true,
    ) {
        //
    }

    /**
     * Mirror Sail's defaults. Windows has no $UID, so the Sail user maps to
     * 1000:1000, which is what Sail's images and Docker Desktop expect.
     *
     * @param  array<string, string>  $processEnv
     * @param  array<string, string>  $dotEnv
     * @return array<string, string>
     */
    public static function resolveEnvironment(array $processEnv, array $dotEnv): array
    {
        $env = array_merge($processEnv, $dotEnv);

        $defaults = [
            'APP_PORT' => '80',
            'APP_SERVICE' => 'laravel.test',
            'APP_USER' => 'sail',
            'DB_PORT' => '3306',
            'WWWUSER' => '1000',
            'WWWGROUP' => '1000',
            'SAIL_FILES' => '',
            'SAIL_SHARE_DASHBOARD' => '4040',
            'SAIL_SHARE_SERVER_HOST' => 'laravel-sail.site',
            'SAIL_SHARE_SERVER_PORT' => '8080',
            'SAIL_SHARE_SUBDOMAIN' => '',
            'SAIL_SHARE_SERVER' => '',
            'SAIL_DOCKER_BINARY' => 'docker',
        ];

        foreach ($defaults as $key => $value) {
            if (! isset($env[$key]) || $env[$key] === '') {
                $env[$key] = $value;
            }
        }

        if (! isset($env['SAIL_SHARE_DOMAIN']) || $env['SAIL_SHARE_DOMAIN'] === '') {
            $env['SAIL_SHARE_DOMAIN'] = $env['SAIL_SHARE_SERVER_HOST'];
        }

        return $env;
    }

    /**
     * SAIL_FILES is colon-separated on Unix; Windows paths contain colons,
     * so on Windows use semicolons (C:\a.yml;C:\b.yml). Colons still work for
     * relative paths.
     *
     * @return list<string>
     */
    public static function composeFiles(string $sailFiles, bool $windows): array
    {
        if (trim($sailFiles) === '') {
            return [];
        }

        $separator = $windows && str_contains($sailFiles, ';') ? ';' : ($windows && preg_match('/^[A-Za-z]:[\\\\\/]/', $sailFiles) ? ';' : ':');

        return array_values(array_filter(array_map('trim', explode($separator, $sailFiles)), fn ($f) => $f !== ''));
    }

    /**
     * Commands that need the containers to be up already.
     */
    public static function requiresRunningContainers(string $command): bool
    {
        return array_key_exists($command, self::APP_COMMANDS) || in_array($command, [
            'bin', 'run', 'docker-compose', 'artisan', 'art', 'a', 'debug', 'dusk', 'dusk:fails',
            'mysql', 'mariadb', 'psql', 'root-shell', 'root-bash', 'mongodb', 'redis', 'valkey', 'share', 'open',
        ], true);
    }

    /**
     * @param  list<string>  $args  Everything after "sail".
     * @return array{type: 'run', command: list<string>}|array{type: 'open', url: string}
     */
    public function build(array $args): array
    {
        $command = $args[0] ?? '';
        $rest = array_slice($args, 1);

        if (isset(self::APP_COMMANDS[$command])) {
            return $this->exec($this->appUserExec(), self::APP_COMMANDS[$command], $rest);
        }

        return match ($command) {
            'bin' => $this->exec($this->appUserExec(), ['./vendor/bin/'.($rest[0] ?? '')], array_slice($rest, 1)),
            'run' => $this->exec($this->appUserExec(), [$rest[0] ?? ''], array_slice($rest, 1)),
            'docker-compose' => $this->exec($this->appUserExec(), $this->compose, $rest),
            'artisan', 'art', 'a' => $this->exec(
                $this->appUserExec(forceNoTty: ($rest[0] ?? '') === 'schedule:run'),
                ['php', 'artisan'],
                $rest,
            ),
            'debug' => $this->exec($this->appUserExec(extraEnv: ['XDEBUG_TRIGGER=1']), ['php', 'artisan'], $rest),
            'dusk', 'dusk:fails' => $this->exec(
                $this->appUserExec(extraEnv: [
                    'APP_URL=http://'.$this->env['APP_SERVICE'],
                    'DUSK_DRIVER_URL=http://selenium:4444/wd/hub',
                ]),
                ['php', 'artisan', $command],
                $rest,
            ),
            'root-shell', 'root-bash' => $this->exec($this->execAs('root'), ['bash'], $rest),
            'mysql' => $this->serviceExec('mysql', ['bash', '-c', 'MYSQL_PWD=${MYSQL_PASSWORD} mysql -u ${MYSQL_USER} ${MYSQL_DATABASE} ${MYSQL_EXTRA_OPTIONS}'], $rest),
            'mariadb' => $this->serviceExec('mariadb', ['bash', '-c', 'MYSQL_PWD=${MYSQL_PASSWORD} mariadb -u ${MYSQL_USER} ${MYSQL_DATABASE}'], $rest),
            'psql' => $this->serviceExec('pgsql', ['bash', '-c', 'PGPASSWORD=${PGPASSWORD} psql -U ${POSTGRES_USER} ${POSTGRES_DB}'], $rest),
            'mongodb' => $this->serviceExec('mongodb', [
                'mongosh', '--port', '27017',
                '--username', $this->env['MONGODB_USERNAME'] ?? '',
                '--password', $this->env['MONGODB_PASSWORD'] ?? '',
                '--authenticationDatabase', 'admin',
            ], $rest),
            'redis' => $this->serviceExec('redis', ['redis-cli'], $rest),
            'valkey' => $this->serviceExec('valkey', ['valkey-cli'], $rest),
            'share' => ['type' => 'run', 'command' => array_merge([
                $this->env['SAIL_DOCKER_BINARY'], 'run', '--init', '--rm',
                '--add-host=host.docker.internal:host-gateway',
                '-p', $this->env['SAIL_SHARE_DASHBOARD'].':4040',
                '-t', 'beyondcodegmbh/expose-server:latest',
                'share', 'http://host.docker.internal:'.$this->env['APP_PORT'],
                '--server-host='.$this->env['SAIL_SHARE_SERVER_HOST'],
                '--server-port='.$this->env['SAIL_SHARE_SERVER_PORT'],
                '--auth='.($this->env['SAIL_SHARE_TOKEN'] ?? ''),
                '--server='.$this->env['SAIL_SHARE_SERVER'],
                '--subdomain='.$this->env['SAIL_SHARE_SUBDOMAIN'],
                '--domain='.$this->env['SAIL_SHARE_DOMAIN'],
            ], $rest)],
            'open' => ['type' => 'open', 'url' => $this->appUrl()],
            // Anything else (up, down, ps, build, logs…) goes to Docker Compose as-is.
            default => ['type' => 'run', 'command' => array_merge($this->compose, $args)],
        };
    }

    /**
     * @return list<string>
     */
    private function appUserExec(bool $forceNoTty = false, array $extraEnv = []): array
    {
        $args = $this->execAs($this->env['APP_USER'], $forceNoTty);

        foreach ($extraEnv as $pair) {
            array_push($args, '-e', $pair);
        }

        foreach (self::AGENT_ENV_VARS as $name) {
            if (($this->env[$name] ?? '') !== '') {
                array_push($args, '-e', $name.'='.$this->env[$name]);
            }
        }

        return $args;
    }

    /**
     * @return list<string>
     */
    private function execAs(string $user, bool $forceNoTty = false): array
    {
        $args = ['exec'];

        if (($this->env['COLORTERM'] ?? '') !== '') {
            array_push($args, '-e', 'COLORTERM='.$this->env['COLORTERM']);
        }

        array_push($args, '-u', $user);

        if ($forceNoTty || ! $this->stdinIsTty) {
            $args[] = '-T';
        }

        return $args;
    }

    /**
     * @param  list<string>  $inner
     * @param  list<string>  $rest
     * @return array{type: 'run', command: list<string>}
     */
    private function serviceExec(string $service, array $inner, array $rest): array
    {
        $args = ['exec'];

        if (! $this->stdinIsTty) {
            $args[] = '-T';
        }

        return ['type' => 'run', 'command' => array_merge($this->compose, $args, [$service], $inner, $rest)];
    }

    /**
     * @param  list<string>  $execArgs
     * @param  list<string>  $inner
     * @param  list<string>  $rest
     * @return array{type: 'run', command: list<string>}
     */
    private function exec(array $execArgs, array $inner, array $rest): array
    {
        return ['type' => 'run', 'command' => array_merge(
            $this->compose, $execArgs, [$this->env['APP_SERVICE']], $inner, $rest,
        )];
    }

    private function appUrl(): string
    {
        $url = rtrim($this->env['APP_URL'] ?? 'http://localhost', '/');
        $port = $this->env['APP_PORT'] ?? '80';

        return ($port !== '' && $port !== '80') ? "{$url}:{$port}" : $url;
    }
}
