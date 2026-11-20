// Package proc runs a program in the foreground the way a shell would, so
// that l4win's shims (php.exe, composer.exe…) behave like the real thing.
package proc

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// Run starts exe with args, sharing this console, and returns its exit code.
//
// Ctrl+C reaches every process attached to the console, so the child gets
// it directly and shuts down its own way; l4win just waits for it. If l4win
// itself is killed (by a task runner or Task Manager), the child and
// everything it started are killed too, instead of being left running.
func Run(exe string, args []string, env []string) int {
	cmd := exec.Command(exe, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), env...)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		for range signals {
			// The child received the same console event; let it decide.
		}
	}()

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "l4win: can't start %s: %v\n", exe, err)
		return 1
	}
	release := bindToLifetime(cmd.Process)
	defer release()

	err := cmd.Wait()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit):
		return exit.ExitCode()
	default:
		fmt.Fprintf(os.Stderr, "l4win: %s: %v\n", exe, err)
		return 1
	}
}
