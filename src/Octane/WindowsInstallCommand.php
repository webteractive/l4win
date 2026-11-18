<?php

namespace Webteractive\L4win\Octane;

use Laravel\Octane\Commands\InstallCommand;

/**
 * `php artisan octane:install --server=frankenphp` downloads the Windows build
 * instead of refusing with "use WSL or Docker".
 */
class WindowsInstallCommand extends InstallCommand
{
    use InstallsWindowsFrankenPhp;
}
