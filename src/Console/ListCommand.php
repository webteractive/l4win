<?php

namespace Webteractive\L4win\Console;

use Illuminate\Console\Command;
use Symfony\Component\Console\Output\OutputInterface;
use Webteractive\L4win\Cli\CommandList;

class ListCommand extends Command
{
    protected $signature = 'l4win';

    protected $description = 'List l4win commands for running Laravel on Windows';

    public function handle(): int
    {
        $text = rtrim(CommandList::render(CommandList::findCli(), 'php vendor\\bin\\l4win'));

        // Raw: the list is plain aligned text, not console markup.
        foreach (explode(PHP_EOL, $text) as $line) {
            $this->output->writeln($line, OutputInterface::OUTPUT_RAW);
        }

        return self::SUCCESS;
    }
}
