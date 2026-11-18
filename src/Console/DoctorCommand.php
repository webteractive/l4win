<?php

namespace Webteractive\L4win\Console;

use Illuminate\Console\Command;
use Webteractive\L4win\Doctor\Check;
use Webteractive\L4win\Doctor\Doctor;

class DoctorCommand extends Command
{
    protected $signature = 'l4win:doctor';

    protected $description = 'Check this machine and project for common Windows problems';

    public function handle(): int
    {
        $checks = (new Doctor($this->laravel->basePath()))->run();

        $this->newLine();

        foreach ($checks as $check) {
            $color = match ($check->status) {
                Check::OK => 'green',
                Check::INFO => 'blue',
                Check::WARN => 'yellow',
                Check::FAIL => 'red',
            };

            $this->line(sprintf('  <fg=%s>%s</> %-24s <fg=gray>%s</>', $color, $check->symbol(), $check->label, $check->value));

            if ($check->hint !== null) {
                $this->line(sprintf('      <fg=gray>→ %s</>', $check->hint));
            }
        }

        $failures = count(array_filter($checks, fn (Check $c) => $c->status === Check::FAIL));
        $warnings = count(array_filter($checks, fn (Check $c) => $c->status === Check::WARN));

        $this->newLine();

        if ($failures > 0) {
            $this->components->error("{$failures} problem(s) and {$warnings} warning(s) found.");

            return self::FAILURE;
        }

        $warnings > 0
            ? $this->components->warn("No blocking problems, {$warnings} warning(s).")
            : $this->components->info('Everything looks good.');

        return self::SUCCESS;
    }
}
