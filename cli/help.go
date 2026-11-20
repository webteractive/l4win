package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/webteractive/l4win/cli/internal/home"
	"github.com/webteractive/l4win/cli/internal/php"
	"github.com/webteractive/l4win/cli/internal/project"
)

type entry struct{ name, summary string }

type group struct {
	title   string
	entries []entry
}

var groups = []group{
	{"Setup", []entry{
		{"setup", "Install PHP, Composer and Node.js and put them on your PATH"},
		{"install", "Install or update a tool: php, php@8.3, node, composer, mysql…"},
		{"uninstall", "Remove a version or service: php@8.2, node@22, mysql"},
		{"use", "Set the default version: php@8.4, node@24"},
		{"list", "Show installed PHP and Node.js versions"},
		{"which", "Show which versions this folder uses, and why"},
	}},
	{"Services", []entry{
		{"up", "Start what this project's .env needs (MySQL, PostgreSQL, Redis…)"},
		{"down", "Stop services"},
		{"status", "Show services, ports and data folders"},
		{"db", "Create a database and user"},
	}},
	{"Sites (https://NAME.localhost)", []entry{
		{"park", "Serve every Laravel app in this folder as https://FOLDER.localhost"},
		{"link", "Serve this project (optionally under another name)"},
		{"unlink", "Stop serving a linked project; `unpark` for a parked folder"},
		{"sites", "List the sites, their PHP versions and folders"},
		{"open", "Open this project's site in the browser"},
		{"trust", "Make browsers trust the local HTTPS certificates (once)"},
	}},
	{"Tools (after setup, also just php, composer, node, npm, npx)", []entry{
		{"php", "PHP, in the version this project asks for"},
		{"composer", "Composer, on the project's PHP"},
		{"node", "Node.js (npm and npx too)"},
	}},
}

// details is the longer help shown by `l4win help COMMAND`.
var details = map[string]string{
	"setup": `l4win setup [--no-path]
l4win setup --undo

Installs PHP, Composer and Node.js into ` + "%LOCALAPPDATA%\\l4win" + `, puts l4win, php,
composer, node, npm and npx in its bin folder and that folder first on your
user PATH (no admin rights). Lists any program that would still run instead
because it's earlier on the PATH.

  --no-path   don't change the PATH
  --undo      stop services and take l4win off the PATH (then delete the folder
              to remove everything)`,
	"install": `l4win install [TOOL...]

With no TOOL: php, composer and node. Each is downloaded from its official
source and checked against its published SHA-256.

  php, php@8.3, php@^8.2   newest matching PHP (older patches are replaced)
  node, node@22, node@lts  Node.js (default: newest LTS)
  composer                 Composer (update it with composer self-update)
  mysql, mariadb,          services (l4win up installs them when needed)
  postgres, redis, mailpit
  phpredis                the redis PHP extension, for every installed PHP
  vcredist                Microsoft Visual C++ runtime, if PHP can't start
  ca                       refresh the CA certificates PHP uses for HTTPS`,
	"uninstall": `l4win uninstall TOOL...

  php@8.2, node@22         remove matching versions
  mysql, mariadb,          remove the service's program; its data is kept
  postgres, redis, mailpit`,
	"use": `l4win use php@8.4 | node@24

Sets the default version. Projects that ask for another one (.php-version,
composer.json, .nvmrc) still get theirs. Installs the version if needed.`,
	"list": `l4win list

Shows installed PHP and Node.js versions and which are the defaults.`,
	"which": `l4win which [php|composer|node]

Shows the version each tool uses in this folder and why. PHP: L4WIN_PHP, then
.php-version, then "require.php" in composer.json, then your default. Node:
L4WIN_NODE, then .nvmrc or .node-version, then your default. A missing version
is installed the first time it's needed.`,
	"up": `l4win up
l4win up SERVICE[:PORT]...

Without arguments, reads the project's .env:
  DB_CONNECTION=mysql    MySQL 8.4 on DB_PORT, creating DB_DATABASE and DB_USERNAME
  DB_CONNECTION=mariadb  MariaDB (newest LTS), the same way
  DB_CONNECTION=pgsql    PostgreSQL 18, the same way (superuser: postgres)
  ...=redis              Redis on REDIS_PORT when CACHE_STORE, QUEUE_CONNECTION,
                         SESSION_DRIVER or BROADCAST_CONNECTION is redis, plus the
                         redis PHP extension. Run by Garnet, Microsoft's open-source
                         Redis-compatible server (with its own private .NET runtime)
  MAIL_MAILER=smtp       Mailpit on MAIL_PORT, inbox at http://localhost:8025

With arguments, starts those services: mysql, mariadb, postgres, redis, mailpit, optionally
with a port (mysql:3307). Services are shared by all projects and keep running
in the background until l4win down. If a port is taken, l4win names the
program holding it.`,
	"down": `l4win down [SERVICE...]

Stops the named services, or all of them. Databases are shut down cleanly.`,
	"status": `l4win status

Shows each service's state, version, port and data folder.`,
	"db": `l4win db create NAME [--user=USER] [--password=PASSWORD] [--service=mysql|mariadb|postgres]

Creates a UTF-8 database on the running MySQL, MariaDB or PostgreSQL. With
--user, also creates that user with full rights on it (on PostgreSQL, as its
owner). Without it, the database belongs to the server's admin user (root,
or postgres). Existing databases and users are left as they are.`,
	"park": `l4win park [FOLDER]
l4win unpark [FOLDER]

Every Laravel app in FOLDER (default: the current one) is served at
https://NAME.localhost, NAME being its folder name in lowercase (spaces and
underscores become dashes). New apps are picked up when the web server starts.`,
	"link": `l4win link [NAME]
l4win unlink [NAME|FOLDER]

Serves the current project at https://NAME.localhost (default: its folder
name), wherever it is. A link wins over a parked app with the same name.
Set APP_URL to the site's address in the project's .env.`,
	"sites": `l4win sites

Lists every site with its PHP version and folder. Each site runs on the PHP
version its project asks for, like the php command does.`,
	"open": "l4win open [NAME]\n\nOpens this project's site (or NAME's) in the browser, starting the web server if needed.",
	"trust": `l4win trust [--undo]

The web server makes its own certificate authority, on this machine only, for
https://*.localhost. This adds it to your user's trusted certificates so
browsers accept the sites (Windows asks you to confirm; no admin rights).
Firefox uses it too, after a restart. --undo removes it.`,
	"web": `l4win up web | l4win down web

Starts or stops the web server: Caddy on ports 443 (and 80, which redirects to
HTTPS), with php-cgi workers for each PHP version the sites use. l4win up in a
project that is a site starts it too.`,
	"php":      "l4win php ARGS...\n\nRuns PHP in the version this folder asks for (see `l4win help which`).",
	"composer": "l4win composer ARGS...\n\nRuns Composer on the project's PHP version.",
	"node":     "l4win node|npm|npx ARGS...\n\nRuns Node.js tools in the version this folder asks for.",
}

func help(args []string) int {
	if len(args) > 0 {
		return helpFor(args[0])
	}

	fmt.Printf("l4win %s: Laravel for Windows, without Docker, WSL or Herd\n", buildVersion)
	for _, g := range groups {
		printGroup(g)
	}

	if root := project.Root(cwd()); root != "" {
		if cmds, ok := projectCommands(findProjectScript(root)); ok {
			printGroup(group{"This project (webteractive/l4win package)", cmds})
		} else if findProjectScript(cwd()) == "" {
			fmt.Println("\nThis project")
			fmt.Println("  The webteractive/l4win package adds doctor, queues and sail here:")
			fmt.Println("  composer require --dev webteractive/l4win")
		}
	}

	fmt.Printf("\nRun `l4win help COMMAND` for details. Everything lives in %s\n", home.Dir())
	return 0
}

func printGroup(g group) {
	fmt.Printf("\n%s\n", g.title)
	for _, e := range g.entries {
		fmt.Printf("  %-10s %s\n", e.name, e.summary)
	}
}

func helpFor(name string) int {
	switch name {
	case "npm", "npx":
		name = "node"
	case "start":
		name = "up"
	case "stop":
		name = "down"
	case "ls":
		name = "list"
	case "unpark", "forget":
		name = "park"
	case "unlink":
		name = "link"
	}
	if text, ok := details[name]; ok {
		fmt.Println(text)
		return 0
	}
	// The package's own commands explain themselves.
	if script := findProjectScript(cwd()); script != "" {
		return runPHP([]string{script, "help", name})
	}
	fmt.Fprintf(os.Stderr, "l4win: no command %q. Run `l4win` for the list.\n", name)
	return 1
}

// findProjectScript returns the package's bin/l4win in the project around
// dir, or "".
func findProjectScript(dir string) string {
	for {
		script := filepath.Join(dir, "vendor", "webteractive", "l4win", "bin", "l4win")
		if home.Exists(script) {
			return script
		}
		if home.Exists(filepath.Join(dir, "composer.json")) {
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// projectCommands asks the project's package for its commands. It only
// uses a PHP that's already installed: listing commands never downloads.
func projectCommands(script string) ([]entry, bool) {
	if script == "" {
		return nil, false
	}
	v, ok := php.Default(php.Installed())
	if !ok {
		return nil, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, php.Exe(v), script, "commands")
	// script is <project>/vendor/webteractive/l4win/bin/l4win.
	cmd.Dir = filepath.Join(filepath.Dir(script), "..", "..", "..", "..")
	cmd.Env = append(os.Environ(), php.Env(v)...)
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	var cmds []entry
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		name, summary, ok := strings.Cut(sc.Text(), "\t")
		if !ok || name == "" {
			continue
		}
		if _, ours := commands[name]; ours {
			continue // l4win.exe's own command wins
		}
		cmds = append(cmds, entry{name, strings.TrimSpace(summary)})
	}
	return cmds, len(cmds) > 0
}
