<?php

namespace Webteractive\L4win\Sail;

/**
 * Small .env reader for the Sail launcher, which must work before Laravel
 * boots. Handles comments, `export`, single/double quotes, escapes in double
 * quotes and ${VAR} references, which covers what Sail's bash `source .env` sees
 * in Laravel projects.
 */
final class DotEnv
{
    /**
     * @param  array<string, string>  $environment  Values available to ${VAR} references.
     * @return array<string, string>
     */
    public static function parse(string $contents, array $environment = []): array
    {
        $values = [];

        foreach (preg_split('/\r\n|\n|\r/', $contents) ?: [] as $line) {
            $line = trim($line);

            if ($line === '' || str_starts_with($line, '#')) {
                continue;
            }

            if (str_starts_with($line, 'export ')) {
                $line = ltrim(substr($line, 7));
            }

            if (! preg_match('/^([A-Za-z_][A-Za-z0-9_.]*)\s*=\s*(.*)$/', $line, $match)) {
                continue;
            }

            [$name, $raw] = [$match[1], $match[2]];

            $values[$name] = self::value($raw, $values + $environment);
        }

        return $values;
    }

    /**
     * @param  array<string, string>  $known
     */
    private static function value(string $raw, array $known): string
    {
        if ($raw === '') {
            return '';
        }

        if ($raw[0] === "'") {
            $end = strpos($raw, "'", 1);

            return $end === false ? substr($raw, 1) : substr($raw, 1, $end - 1);
        }

        if ($raw[0] === '"') {
            $value = '';
            $length = strlen($raw);

            for ($i = 1; $i < $length; $i++) {
                $char = $raw[$i];

                if ($char === '\\' && $i + 1 < $length) {
                    $next = $raw[++$i];
                    $value .= match ($next) {
                        'n' => "\n",
                        't' => "\t",
                        default => $next,
                    };

                    continue;
                }

                if ($char === '"') {
                    break;
                }

                $value .= $char;
            }

            return self::expand($value, $known);
        }

        // Unquoted: an inline comment starts at " #".
        $value = preg_replace('/\s+#.*$/', '', $raw) ?? $raw;

        return self::expand(trim($value), $known);
    }

    /**
     * @param  array<string, string>  $known
     */
    private static function expand(string $value, array $known): string
    {
        return preg_replace_callback(
            '/\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}/',
            static function (array $m) use ($known): string {
                $current = $known[$m[1]] ?? '';

                return $current !== '' ? $current : ($m[2] ?? '');
            },
            $value,
        ) ?? $value;
    }
}
