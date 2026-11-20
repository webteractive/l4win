package php

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/webteractive/l4win/cli/internal/fetch"
	"github.com/webteractive/l4win/cli/internal/home"
)

// PHP for Windows is built with Visual C++ and needs its runtime DLLs.
// Most machines already have them; a fresh Windows install may not.
const vcRedistURL = "https://aka.ms/vs/17/release/vc_redist.x64.exe"

// MissingVCRuntime reports whether the x64 Visual C++ runtime is absent.
func MissingVCRuntime() bool {
	sys := filepath.Join(os.Getenv("SystemRoot"), "System32")
	for _, dll := range []string{"vcruntime140.dll", "vcruntime140_1.dll", "msvcp140.dll"} {
		if !home.Exists(filepath.Join(sys, dll)) {
			return true
		}
	}
	return false
}

// InstallVCRedist downloads Microsoft's installer and runs it. Windows asks
// for administrator permission; this is the only step that does.
func InstallVCRedist() error {
	exe := home.Path("cache", "vc_redist.x64.exe")
	if err := fetch.File(vcRedistURL, exe, "", "vc_redist.x64.exe"); err != nil {
		return err
	}
	defer os.Remove(exe)
	fmt.Fprintln(os.Stderr, "Running Microsoft's Visual C++ Redistributable installer (Windows will ask for permission)...")
	cmd := exec.Command(exe, "/install", "/passive", "/norestart")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		// 1638: a newer version is already installed; 3010: reboot needed.
		if ee, ok := err.(*exec.ExitError); ok && (ee.ExitCode() == 1638 || ee.ExitCode() == 3010) {
			return nil
		}
		return fmt.Errorf("the Visual C++ Redistributable installer failed: %w", err)
	}
	return nil
}
