<?php

namespace Webteractive\L4win\Octane;

use Webteractive\L4win\Platform;

/**
 * Octane's server commands subscribe to SIGINT/SIGTERM/SIGHUP. Those constants
 * only exist with pcntl, so on Windows the command dies before it starts.
 * Without pcntl we subscribe to nothing and stop the server on Ctrl+C through
 * Windows' own console control handler instead.
 */
trait HandlesWindowsSignals
{
    public function getSubscribedSignals(): array
    {
        return Platform::hasPcntl() ? parent::getSubscribedSignals() : [];
    }

    protected function stopServerOnCtrlC(): void
    {
        if (! function_exists('sapi_windows_set_ctrl_handler')) {
            return;
        }

        @sapi_windows_set_ctrl_handler(function (): void {
            $this->stopServer();

            exit(0);
        });
    }
}
