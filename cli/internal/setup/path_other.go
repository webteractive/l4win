//go:build !windows

package setup

import (
	"errors"
	"os"
)

var errWindowsOnly = errors.New("changing the PATH is only supported on Windows")

func AddToUserPath(string) (bool, error)      { return false, errWindowsOnly }
func RemoveFromUserPath(string) (bool, error) { return false, errWindowsOnly }
func EffectivePath() string                   { return os.Getenv("PATH") }
