<?php
// Minimal stand-ins with the same method signatures as Laravel, Pail 1.x and
// Octane 2.x, so PHP can check our subclasses are signature-compatible.

namespace Illuminate\Console { abstract class Command {
    public $components; protected $laravel; protected $output;
    public function option($key = null) { return null; }
    public function line($s) {} public function newLine($n = 1) { return $this; }
    public function trap($signals, $callback) {}
}}

namespace Illuminate\Support { abstract class ServiceProvider {
    protected $app; public function commands($c) {}
}}

namespace Laravel\Pail { class ProcessFactory {} class File { public function __construct(protected string $file) {} public function create(): void {} public function __toString(): string { return $this->file; } } }

namespace Laravel\Pail\Console\Commands { class PailCommand extends \Illuminate\Console\Command {
    protected ?\Laravel\Pail\File $file = null;
    public function handle(\Laravel\Pail\ProcessFactory $processFactory): void {}
    public function __destruct() {}
}}

namespace Laravel\Octane\FrankenPhp { class ServerProcessInspector {} class ServerStateFile {} }

namespace Laravel\Octane\Commands\Concerns {
    trait InteractsWithServers {
        public function getSubscribedSignals(): array { return [SIGINT]; }
        public function handleSignal(int $signal, int|false $previousExitCode = 0): int|false { return 0; }
    }
    trait InstallsFrankenPhpDependencies {
        protected function findFrankenPhpBinary(): ?string { return null; }
        protected function ensureFrankenPhpBinaryIsInstalled() { return 'frankenphp'; }
        protected function downloadFrankenPhpBinary() { return 'frankenphp'; }
    }
}

namespace Laravel\Octane\Commands {
    class StartCommand extends \Illuminate\Console\Command {
        use Concerns\InteractsWithServers;
        public function handle() { return 0; }
        protected function stopServer() {}
    }
    class StartFrankenPhpCommand extends \Illuminate\Console\Command {
        use Concerns\InstallsFrankenPhpDependencies, Concerns\InteractsWithServers;
        public function handle(\Laravel\Octane\FrankenPhp\ServerProcessInspector $inspector, \Laravel\Octane\FrankenPhp\ServerStateFile $serverStateFile) { return 0; }
        protected function stopServer() {}
    }
    class InstallCommand extends \Illuminate\Console\Command {
        use Concerns\InstallsFrankenPhpDependencies;
    }
}
