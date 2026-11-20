//go:build !windows

package proc

import "os"

// TieChildrenToMe does nothing outside Windows.
func TieChildrenToMe() error { return nil }

// l4win targets Windows; elsewhere children run without a job.
func bindToLifetime(*os.Process) func() { return func() {} }
