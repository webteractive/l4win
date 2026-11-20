<?php

namespace Webteractive\L4win\Cli;

/**
 * The package's commands, shown by `vendor\bin\l4win`, `php artisan l4win`,
 * and (through `vendor\bin\l4win commands`) by l4win.exe's own list.
 */
final class CommandList
{
    /**
     * @return list<array{0: string, 1: string}> [name, description]
     */
    public static function package(): array
    {
        return [
            ['doctor', 'Check PHP, extensions, tools and this project for Windows problems'],
            ['queues', 'Run the queues in config/horizon.php natively (php artisan l4win:queues)'],
            ['sail', 'Laravel Sail without bash or WSL (needs Docker Desktop)'],
        ];
    }

    /**
     * Tab-separated "name<TAB>description" lines, read by l4win.exe.
     */
    public static function machineReadable(): string
    {
        return implode('', array_map(fn ($c) => $c[0]."\t".$c[1]."\n", self::package()));
    }

    /**
     * The grouped list for people. $prefix is how they run the package's
     * script, used in the hints.
     */
    public static function render(?string $cliPath, string $prefix = 'vendor\\bin\\l4win'): string
    {
        $lines = ['l4win: Laravel for Windows', '', 'This project (webteractive/l4win package)'];

        foreach (self::package() as [$name, $description]) {
            $lines[] = sprintf('  %-12s %s', $name, $description);
        }

        $lines[] = '';
        $lines[] = 'Run them as `'.$prefix.' COMMAND`. Details: `'.$prefix.' help sail`.';
        $lines[] = 'Artisan: php artisan l4win:doctor, php artisan l4win:queues';
        $lines[] = '';
        $lines[] = 'l4win.exe (PHP, Composer, Node.js, MySQL, MariaDB and Mailpit without Docker, WSL or Herd)';

        if ($cliPath !== null) {
            $lines[] = '  Installed: '.$cliPath;
            $lines[] = '  Type `l4win` in any terminal to see all its commands, including the ones above.';
        } elseif (PHP_OS_FAMILY === 'Windows') {
            $lines[] = '  Not installed. Install it with `'.$prefix.' install-cli`; then `l4win` works in any terminal.';
        } else {
            $lines[] = '  Windows only; nothing to install on '.PHP_OS_FAMILY.'.';
        }

        return implode(PHP_EOL, $lines).PHP_EOL;
    }

    /**
     * Where l4win.exe is, if it's installed.
     */
    public static function findCli(): ?string
    {
        if (PHP_OS_FAMILY !== 'Windows') {
            return null;
        }

        $candidates = [];

        if (($home = getenv('L4WIN_HOME')) !== false && $home !== '') {
            $candidates[] = $home.'\\bin\\l4win.exe';
        }

        if (($local = getenv('LOCALAPPDATA')) !== false && $local !== '') {
            $candidates[] = $local.'\\l4win\\bin\\l4win.exe';
        }

        foreach (explode(PATH_SEPARATOR, (string) getenv('PATH')) as $dir) {
            if ($dir !== '') {
                $candidates[] = rtrim($dir, '\\/').'\\l4win.exe';
            }
        }

        foreach ($candidates as $file) {
            if (is_file($file)) {
                return $file;
            }
        }

        return null;
    }
}
