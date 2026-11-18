<?php

namespace Webteractive\L4win;

final class Platform
{
    public static function isWindows(): bool
    {
        return PHP_OS_FAMILY === 'Windows';
    }

    public static function hasPcntl(): bool
    {
        return function_exists('pcntl_fork');
    }

    /**
     * Shims only kick in where they're needed, so teammates on WSL, Linux or
     * macOS get stock Laravel behaviour even with this package installed.
     */
    public static function needsShims(): bool
    {
        if (self::flag('L4WIN_FORCE', false)) {
            return true;
        }

        return self::isWindows() || ! self::hasPcntl();
    }

    public static function flag(string $name, bool $default): bool
    {
        $value = $_ENV[$name] ?? $_SERVER[$name] ?? getenv($name);

        if ($value === false || $value === null || $value === '') {
            return $default;
        }

        return filter_var($value, FILTER_VALIDATE_BOOLEAN, FILTER_NULL_ON_FAILURE) ?? $default;
    }

    public static function isProcessAlive(int $pid): bool
    {
        if ($pid <= 0) {
            return false;
        }

        if (self::isWindows()) {
            $output = @shell_exec('tasklist /FI "PID eq '.$pid.'" /NH 2>NUL');

            // If tasklist can't run, assume alive so a live session's file is never deleted.
            return ! is_string($output) || preg_match('/\b'.$pid.'\b/', $output) === 1;
        }

        if (function_exists('posix_kill')) {
            return @posix_kill($pid, 0);
        }

        return is_dir('/proc') ? is_dir("/proc/{$pid}") : true;
    }

    public static function which(string $binary): ?string
    {
        $command = self::isWindows()
            ? 'where '.escapeshellarg($binary).' 2>NUL'
            : 'command -v '.escapeshellarg($binary).' 2>/dev/null';

        $output = @shell_exec($command);

        if (! is_string($output) || trim($output) === '') {
            return null;
        }

        return trim((string) strtok($output, "\r\n"));
    }
}
