<?php

namespace Webteractive\L4win\Octane;

use Laravel\Octane\Commands\StartFrankenPhpCommand;
use Laravel\Octane\FrankenPhp\ServerProcessInspector;
use Laravel\Octane\FrankenPhp\ServerStateFile;

class WindowsStartFrankenPhpCommand extends StartFrankenPhpCommand
{
    use HandlesWindowsSignals;
    use InstallsWindowsFrankenPhp;

    public function handle(ServerProcessInspector $inspector, ServerStateFile $serverStateFile)
    {
        $this->stopServerOnCtrlC();

        return parent::handle($inspector, $serverStateFile);
    }
}
