//go:build !windows

package services

import "errors"

var errWindowsOnly = errors.New("services are only supported on Windows")

func startDetached(string, []string, string, []string) (int, error) { return 0, errWindowsOnly }
func processAlive(int, string) bool                                 { return false }
func killProcess(int) error                                         { return errWindowsOnly }
func portHolder(int) string                                         { return "" }
