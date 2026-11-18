<?php

namespace Webteractive\L4win\Doctor;

final class Check
{
    public const OK = 'ok';

    public const INFO = 'info';

    public const WARN = 'warn';

    public const FAIL = 'fail';

    private function __construct(
        public readonly string $status,
        public readonly string $label,
        public readonly string $value,
        public readonly ?string $hint = null,
    ) {
        //
    }

    public static function ok(string $label, string $value): self
    {
        return new self(self::OK, $label, $value);
    }

    public static function info(string $label, string $value, ?string $hint = null): self
    {
        return new self(self::INFO, $label, $value, $hint);
    }

    public static function warn(string $label, string $value, string $hint): self
    {
        return new self(self::WARN, $label, $value, $hint);
    }

    public static function fail(string $label, string $value, string $hint): self
    {
        return new self(self::FAIL, $label, $value, $hint);
    }

    public function symbol(): string
    {
        return match ($this->status) {
            self::OK => '✓',
            self::INFO => 'i',
            self::WARN => '!',
            self::FAIL => '✗',
        };
    }
}
