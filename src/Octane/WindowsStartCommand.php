<?php

namespace Webteractive\L4win\Octane;

use Laravel\Octane\Commands\StartCommand;
use Webteractive\L4win\Platform;

class WindowsStartCommand extends StartCommand
{
    use HandlesWindowsSignals;

    public function handle()
    {
        $server = $this->option('server') ?: config('octane.server');

        if (! Platform::hasPcntl() && $server !== 'frankenphp') {
            $this->components->error("Octane's [{$server}] server can't run natively on Windows.");
            $this->line('  Use FrankenPHP, which has an official Windows build: set <fg=yellow>OCTANE_SERVER=frankenphp</> in .env');
            $this->line('  or run <fg=yellow>php artisan octane:start --server=frankenphp</>. Swoole needs WSL or Docker.');

            return 1;
        }

        $this->stopServerOnCtrlC();

        return parent::handle();
    }
}
