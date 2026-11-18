<?php

namespace Webteractive\L4win\Support;

/**
 * A small, pure-PHP replacement for `tail -F`.
 *
 * It remembers a byte offset, returns only complete new lines on each read,
 * and starts over if the file is truncated, deleted or recreated.
 */
final class FileTailer
{
    private int $offset = 0;

    private string $buffer = '';

    public function __construct(private readonly string $path, bool $startAtEnd = true)
    {
        if ($startAtEnd && is_file($path)) {
            clearstatcache(true, $path);
            $this->offset = (int) filesize($path);
        }
    }

    /**
     * @return list<string> Complete, non-empty lines appended since the last read.
     */
    public function read(): array
    {
        clearstatcache(true, $this->path);

        if (! is_file($this->path)) {
            $this->reset();

            return [];
        }

        $size = @filesize($this->path);

        if ($size === false) {
            return [];
        }

        if ($size < $this->offset) {
            $this->reset();
        }

        if ($size === $this->offset) {
            return [];
        }

        $handle = @fopen($this->path, 'rb');

        if ($handle === false) {
            return [];
        }

        try {
            fseek($handle, $this->offset);
            $chunk = stream_get_contents($handle);
        } finally {
            fclose($handle);
        }

        if (! is_string($chunk) || $chunk === '') {
            return [];
        }

        $this->offset += strlen($chunk);

        $lines = explode("\n", $this->buffer.$chunk);
        $this->buffer = (string) array_pop($lines);

        $lines = array_map(static fn (string $line): string => rtrim($line, "\r"), $lines);

        return array_values(array_filter($lines, static fn (string $line): bool => $line !== ''));
    }

    private function reset(): void
    {
        $this->offset = 0;
        $this->buffer = '';
    }
}
