<?php

namespace Webteractive\L4win;

use Illuminate\Support\ServiceProvider;
use Laravel\Octane\Commands\InstallCommand as OctaneInstallCommand;
use Laravel\Octane\Commands\StartCommand as OctaneStartCommand;
use Laravel\Octane\Commands\StartFrankenPhpCommand as OctaneStartFrankenPhpCommand;
use Laravel\Pail\Console\Commands\PailCommand;
use Webteractive\L4win\Console\DoctorCommand;
use Webteractive\L4win\Console\QueuesCommand;
use Webteractive\L4win\Console\WindowsPailCommand;
use Webteractive\L4win\Octane\WindowsInstallCommand;
use Webteractive\L4win\Octane\WindowsStartCommand;
use Webteractive\L4win\Octane\WindowsStartFrankenPhpCommand;

class L4winServiceProvider extends ServiceProvider
{
    public function register(): void
    {
        if (! Platform::needsShims()) {
            return;
        }

        // Artisan resolves these commands from the container, so swapping the
        // bindings replaces them everywhere (`composer run dev`, `octane:start`
        // calling `octane:frankenphp`, …) without editing the project.
        // Each swap only happens when the package being patched is installed.

        if (class_exists(PailCommand::class) && Platform::flag('L4WIN_PAIL', true)) {
            $this->app->bind(PailCommand::class, WindowsPailCommand::class);
        }

        if (class_exists(OctaneStartFrankenPhpCommand::class) && Platform::flag('L4WIN_OCTANE', true)) {
            $this->app->bind(OctaneStartCommand::class, WindowsStartCommand::class);
            $this->app->bind(OctaneStartFrankenPhpCommand::class, WindowsStartFrankenPhpCommand::class);
            $this->app->bind(OctaneInstallCommand::class, WindowsInstallCommand::class);
        }
    }

    public function boot(): void
    {
        if ($this->app->runningInConsole()) {
            $this->commands([
                DoctorCommand::class,
                QueuesCommand::class,
            ]);
        }
    }
}
