package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/webteractive/l4win/cli/internal/composer"
	"github.com/webteractive/l4win/cli/internal/home"
	"github.com/webteractive/l4win/cli/internal/node"
	"github.com/webteractive/l4win/cli/internal/php"
	"github.com/webteractive/l4win/cli/internal/proc"
	"github.com/webteractive/l4win/cli/internal/project"
)

// tools are the commands l4win runs on the project's versions.
var tools = map[string]func([]string) int{
	"php":      runPHP,
	"composer": runComposer,
	"node":     runNode,
	"npm":      func(args []string) int { return runNodeCLI("npm", args) },
	"npx":      func(args []string) int { return runNodeCLI("npx", args) },
}

func cwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

func fail(err error) int {
	fmt.Fprintf(os.Stderr, "l4win: %v\n", err)
	return 1
}

func runPHP(args []string) int {
	v, err := php.Resolve(cwd())
	if err != nil {
		return fail(err)
	}
	return proc.Run(php.Exe(v), args, php.Env(v))
}

func runComposer(args []string) int {
	if !composer.Installed() {
		if err := composer.Install(); err != nil {
			return fail(err)
		}
	}
	v, err := php.Resolve(cwd())
	if err != nil {
		return fail(err)
	}
	return proc.Run(php.Exe(v), append([]string{composer.Phar()}, args...), php.Env(v))
}

func runNode(args []string) int {
	v, err := node.Resolve(cwd())
	if err != nil {
		return fail(err)
	}
	return proc.Run(node.Exe(v), args, nodeEnv(node.Dir(v)))
}

func runNodeCLI(tool string, args []string) int {
	v, err := node.Resolve(cwd())
	if err != nil {
		return fail(err)
	}
	return proc.Run(node.Exe(v), append([]string{node.CLI(v, tool)}, args...), nodeEnv(node.Dir(v)))
}

// nodeEnv puts the real node.exe first on PATH for npm scripts and the
// packages they run, the way Node's own installer does.
func nodeEnv(dir string) []string {
	return []string{"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH")}
}

// knownPackageCommands are forwarded even when the package's own list
// can't be read (no PHP installed yet: running them installs it).
var knownPackageCommands = map[string]bool{"doctor": true, "sail": true, "queues": true}

// forwardToProject runs the project's webteractive/l4win package CLI for
// the commands it lists (doctor, queues, sail…).
func forwardToProject(cmd string, args []string) (int, bool) {
	script := findProjectScript(cwd())
	if script == "" {
		if knownPackageCommands[cmd] && project.Root(cwd()) != "" {
			fmt.Fprintf(os.Stderr, "l4win: `%s` comes from the webteractive/l4win Composer package, which this project doesn't have.\n"+
				"Add it with: composer require --dev webteractive/l4win\n", cmd)
			return 1, true
		}
		return 0, false
	}

	known := knownPackageCommands[cmd]
	if cmds, ok := projectCommands(script); ok {
		known = false
		for _, c := range cmds {
			if c.name == cmd {
				known = true
			}
		}
	}
	if !known {
		return 0, false
	}
	return runPHP(append([]string{script, cmd}, args...)), true
}

func homeDir() string { return home.Dir() }

// splitTool turns "php@8.3" into ("php", "8.3").
func splitTool(arg string) (string, string) {
	name, ver, _ := strings.Cut(strings.ToLower(arg), "@")
	return name, ver
}
