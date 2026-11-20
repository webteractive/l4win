// l4win manages everything a Laravel project needs on Windows: PHP,
// Composer and Node.js, downloaded from their official sources into one
// folder. No Docker, WSL, Herd or XAMPP.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/webteractive/l4win/cli/internal/fetch"
)

// buildVersion is set at build time: -ldflags "-X main.buildVersion=0.1.0".
var buildVersion = "dev"

func main() {
	fetch.Version = buildVersion

	// Called as php.exe, composer.exe, node.exe…: act as that tool.
	name := strings.ToLower(filepath.Base(os.Args[0]))
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if run, ok := tools[name]; ok {
		os.Exit(run(os.Args[1:]))
	}

	args := os.Args[1:]
	if len(args) == 0 {
		os.Exit(help(nil))
	}
	cmd, rest := args[0], args[1:]

	if run, ok := tools[cmd]; ok {
		os.Exit(run(rest))
	}
	if run, ok := commands[cmd]; ok {
		os.Exit(run(rest))
	}
	// The PHP package's own commands (doctor, sail) when the project has it.
	if code, ok := forwardToProject(cmd, rest); ok {
		os.Exit(code)
	}
	fmt.Fprintf(os.Stderr, "l4win: unknown command %q\n\n", cmd)
	help(nil)
	os.Exit(1)
}

var commands map[string]func([]string) int

func init() {
	commands = map[string]func([]string) int{
		"setup":        cmdSetup,
		"install":      cmdInstall,
		"uninstall":    cmdUninstall,
		"list":         cmdList,
		"ls":           cmdList,
		"use":          cmdUse,
		"which":        cmdWhich,
		"up":           cmdUp,
		"start":        cmdUp,
		"down":         cmdDown,
		"stop":         cmdDown,
		"status":       cmdStatus,
		"db":           cmdDB,
		"park":         cmdPark,
		"unpark":       cmdUnpark,
		"forget":       cmdUnpark,
		"link":         cmdLink,
		"unlink":       cmdUnlink,
		"sites":        cmdSites,
		"open":         cmdOpen,
		"trust":        cmdTrust,
		"__web-daemon": webDaemon,
		"help":         help,
		"-h":           help,
		"--help":       help,
		"version":      cmdVersion,
		"--version":    cmdVersion,
		"-v":           cmdVersion,
	}
}

func cmdVersion([]string) int {
	fmt.Println("l4win " + buildVersion)
	return 0
}
