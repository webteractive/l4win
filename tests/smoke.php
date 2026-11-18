<?php
// Dependency-free tests: php tests/smoke.php
spl_autoload_register(function ($c) {
    $p = 'Webteractive\\L4win\\';
    if (str_starts_with($c, $p)) require __DIR__.'/../src/'.str_replace('\\', '/', substr($c, strlen($p))).'.php';
});

use Webteractive\L4win\Doctor\Doctor;
use Webteractive\L4win\Queues\HorizonPlan;
use Webteractive\L4win\Sail\DotEnv;
use Webteractive\L4win\Sail\SailCommandBuilder;
use Webteractive\L4win\Support\FileTailer;

$failed = false;
function check(bool $ok, string $msg) { global $failed; echo ($ok ? 'PASS ' : 'FAIL ').$msg.PHP_EOL; $failed = $failed || ! $ok; }

// --- FileTailer -----------------------------------------------------------
$f = sys_get_temp_dir().'/lw-'.uniqid().'.pail';
file_put_contents($f, "{\"old\":1}\n");
$t = new FileTailer($f);
check($t->read() === [], 'tailer starts at end of existing file');
file_put_contents($f, "{\"a\":1}\r\n{\"b\":", FILE_APPEND);
check($t->read() === ['{"a":1}'], 'tailer returns complete lines, strips CR, buffers partial');
file_put_contents($f, "2}\n", FILE_APPEND);
check($t->read() === ['{"b":2}'], 'tailer completes buffered line');
file_put_contents($f, "{\"c\":3}\n");
check($t->read() === ['{"c":3}'], 'tailer handles truncation');
unlink($f);
check($t->read() === [], 'tailer handles deletion');
file_put_contents($f, "{\"d\":4}\n");
check($t->read() === ['{"d":4}'], 'tailer handles recreation');
unlink($f);

// --- DotEnv ---------------------------------------------------------------
$env = DotEnv::parse(<<<'ENV'
# comment
APP_NAME="My App"
APP_URL=http://localhost # trailing comment
export APP_PORT=8080
DB_PASSWORD='p@ss#word'
GREETING="line1\nline2"
FULL=${APP_URL}/api
FALLBACK=${MISSING:-default}
EMPTY=
ENV);
check($env['APP_NAME'] === 'My App', 'dotenv: double quotes');
check($env['APP_URL'] === 'http://localhost', 'dotenv: inline comment stripped');
check($env['APP_PORT'] === '8080', 'dotenv: export prefix');
check($env['DB_PASSWORD'] === 'p@ss#word', 'dotenv: single quotes kept literal');
check($env['GREETING'] === "line1\nline2", 'dotenv: escapes in double quotes');
check($env['FULL'] === 'http://localhost/api', 'dotenv: ${VAR} expansion');
check($env['FALLBACK'] === 'default', 'dotenv: ${VAR:-default}');
check($env['EMPTY'] === '', 'dotenv: empty value');

// --- Sail -----------------------------------------------------------------
$sailEnv = SailCommandBuilder::resolveEnvironment(['CLAUDECODE' => '1'], ['APP_PORT' => '8080', 'APP_URL' => 'http://localhost']);
check($sailEnv['WWWUSER'] === '1000' && $sailEnv['APP_SERVICE'] === 'laravel.test', 'sail: Windows defaults (WWWUSER 1000)');
$b = new SailCommandBuilder($sailEnv, ['docker', 'compose'], true);
$dc = ['docker', 'compose'];
check($b->build(['up', '-d'])['command'] === [...$dc, 'up', '-d'], 'sail: unknown commands pass to compose');
check($b->build(['artisan', 'migrate'])['command'] === [...$dc, 'exec', '-u', 'sail', '-e', 'CLAUDECODE=1', 'laravel.test', 'php', 'artisan', 'migrate'], 'sail: artisan as sail user, agent env forwarded');
check(in_array('-T', $b->build(['artisan', 'schedule:run'])['command'], true), 'sail: schedule:run forces -T like Sail');
check($b->build(['bin', 'phpstan', 'analyse'])['command'] === [...$dc, 'exec', '-u', 'sail', '-e', 'CLAUDECODE=1', 'laravel.test', './vendor/bin/phpstan', 'analyse'], 'sail: bin');
check($b->build(['root-shell'])['command'] === [...$dc, 'exec', '-u', 'root', 'laravel.test', 'bash'], 'sail: root-shell');
check(in_array('-e', $b->build(['debug', 'queue:work'])['command'], true) && in_array('XDEBUG_TRIGGER=1', $b->build(['debug', 'queue:work'])['command'], true), 'sail: debug sets XDEBUG_TRIGGER');
check($b->build(['redis'])['command'] === [...$dc, 'exec', 'redis', 'redis-cli'], 'sail: redis');
check($b->build(['open']) === ['type' => 'open', 'url' => 'http://localhost:8080'], 'sail: open adds non-80 port');
$noTty = new SailCommandBuilder($sailEnv, $dc, false);
check(in_array('-T', $noTty->build(['php', '-v'])['command'], true), 'sail: -T when stdin is not a TTY');
check(SailCommandBuilder::composeFiles('C:\\a.yml;C:\\b.yml', true) === ['C:\\a.yml', 'C:\\b.yml'], 'sail: SAIL_FILES with ; on Windows');
check(SailCommandBuilder::composeFiles('docker-compose.yml:docker-compose.dev.yml', false) === ['docker-compose.yml', 'docker-compose.dev.yml'], 'sail: SAIL_FILES with : elsewhere');
check(SailCommandBuilder::requiresRunningContainers('artisan') && ! SailCommandBuilder::requiresRunningContainers('up'), 'sail: running-state requirements');

// --- Horizon plan -----------------------------------------------------------
$horizon = [
    'defaults' => ['supervisor-1' => ['connection' => 'redis', 'queue' => ['default'], 'maxProcesses' => 1, 'memory' => 128, 'tries' => 1, 'timeout' => 60]],
    'environments' => [
        'production' => ['supervisor-1' => ['maxProcesses' => 10]],
        'local' => ['supervisor-1' => ['maxProcesses' => 3], 'emails' => ['connection' => 'redis', 'queue' => 'mail,notifications', 'processes' => 2, 'tries' => 3, 'backoff' => [5, 30]]],
        'staging-*' => ['supervisor-1' => ['maxProcesses' => 2]],
        'off' => ['supervisor-1' => ['maxProcesses' => 0]],
    ],
];
$plan = new HorizonPlan($horizon, ['redis' => ['queue' => 'default']]);
$local = $plan->supervisors('local');
check(count($local) === 2 && $local[0]['processes'] === 3 && $local[0]['tries'] === 1, 'horizon: defaults merged into environment');
check($local[1]['queues'] === ['mail', 'notifications'] && $local[1]['backoff'] === '5,30' && $local[1]['processes'] === 2, 'horizon: queue string, backoff array, processes');
check($plan->supervisors('staging-eu')[0]['processes'] === 2, 'horizon: wildcard environment like Str::is');
check($plan->supervisors('off') === [], 'horizon: maxProcesses 0 skipped like Horizon');
check($plan->supervisors('unknown') === [], 'horizon: unknown environment has no supervisors');
check($plan->supervisors('production', 2)[0]['processes'] === 2, 'horizon: --max-processes cap');
$cmd = HorizonPlan::command($local[1], 'php', false);
check($cmd === ['php', 'artisan', 'queue:listen', 'redis', '--name=emails', '--queue=mail,notifications', '--tries=3', '--timeout=60', '--memory=128', '--sleep=3', '--backoff=5,30'], 'horizon: queue:listen command');

// --- Doctor -----------------------------------------------------------------
$checks = (new Doctor(null))->run();
check(count($checks) > 3, 'doctor runs ('.count($checks).' checks)');

exit($failed ? 1 : 0);
