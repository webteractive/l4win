<?php
// php tests/compat.php — loads every subclass against signature stubs.
require __DIR__.'/stubs/framework.php';

spl_autoload_register(function ($c) {
    $p = 'Webteractive\\L4win\\';
    if (str_starts_with($c, $p)) require __DIR__.'/../src/'.str_replace('\\', '/', substr($c, strlen($p))).'.php';
});

$classes = [
    Webteractive\L4win\Console\WindowsPailCommand::class,
    Webteractive\L4win\Octane\WindowsStartCommand::class,
    Webteractive\L4win\Octane\WindowsStartFrankenPhpCommand::class,
    Webteractive\L4win\Octane\WindowsInstallCommand::class,
    Webteractive\L4win\Console\QueuesCommand::class,
    Webteractive\L4win\Console\DoctorCommand::class,
    Webteractive\L4win\L4winServiceProvider::class,
];

foreach ($classes as $class) {
    class_exists($class);
    echo "PASS loads {$class}\n";
}

// Without pcntl, Octane's signal subscription must be empty instead of touching SIGINT.
$cmd = (new ReflectionClass(Webteractive\L4win\Octane\WindowsStartFrankenPhpCommand::class))->newInstanceWithoutConstructor();
$signals = $cmd->getSubscribedSignals();
echo (function_exists('pcntl_fork') ? $signals === [SIGINT] : $signals === []) ? "PASS getSubscribedSignals respects pcntl availability\n" : "FAIL getSubscribedSignals\n";
