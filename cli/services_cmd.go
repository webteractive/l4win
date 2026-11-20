package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/webteractive/l4win/cli/internal/home"
	"github.com/webteractive/l4win/cli/internal/php"
	"github.com/webteractive/l4win/cli/internal/project"
	"github.com/webteractive/l4win/cli/internal/services"
	"github.com/webteractive/l4win/cli/internal/sites"
)

// cmdUp starts the services the project's .env asks for, or the ones named.
func cmdUp(args []string) int {
	var needs []project.Need
	var notes []string
	phpRedis := false
	root := ""

	if len(args) > 0 {
		for _, arg := range args {
			name, portStr, _ := strings.Cut(arg, ":") // "mysql" or "mysql:3307"
			s, ok := services.Get(name)
			if !ok {
				return fail(fmt.Errorf("unknown service %q (available: %s)", name, services.Names()))
			}
			port := s.DefaultPort
			if portStr != "" {
				p, err := strconv.Atoi(portStr)
				if err != nil {
					return fail(fmt.Errorf("invalid port in %q", arg))
				}
				port = p
			}
			needs = append(needs, project.Need{Service: s.Name, Port: port})
		}
	} else {
		root = project.Root(cwd())
		if root == "" {
			return fail(fmt.Errorf("not in a Laravel project. Name the services instead: l4win up mysql mailpit"))
		}
		envFile := filepath.Join(root, ".env")
		env, err := project.ReadEnv(envFile)
		if err != nil {
			return fail(fmt.Errorf("reading %s: %w", envFile, err))
		}
		plan := project.PlanFor(env)
		needs, notes, phpRedis = plan.Needs, plan.Notes, plan.PHPRedis
		if _, isSite := sites.For(home.LoadConfig(), root); len(needs) == 0 && !isSite {
			fmt.Println("This project's .env doesn't need any services l4win runs (SQLite and the log mailer need none).")
			fmt.Println("To serve it at https://NAME.localhost, run `l4win link`.")
		}
	}

	failed := false
	for _, n := range needs {
		s, _ := services.Get(n.Service)
		st, err := s.Start(n.Port)
		if err != nil {
			fmt.Fprintf(os.Stderr, "✗ %s: %v\n", s.Title, err)
			failed = true
			continue
		}
		line := fmt.Sprintf("✓ %s %s on 127.0.0.1:%d", s.Title, st.Version, st.Port)
		if st.Port != n.Port {
			who := "this project expects"
			if len(args) > 0 {
				who = "you asked for"
			}
			line += fmt.Sprintf(" (it was already running; %s port %d, so stop it with `l4win down %s` to move it)", who, n.Port, s.Name)
		}
		if n.Service == "mailpit" {
			line += fmt.Sprintf(", inbox at http://localhost:%d", services.MailpitUIPort)
		}
		if n.Database != "" {
			if err := s.EnsureDatabase(n.Database, n.Username, n.Password); err != nil {
				fmt.Println(line)
				fmt.Fprintf(os.Stderr, "✗ creating database %q: %v\n", n.Database, err)
				failed = true
				continue
			}
			line += fmt.Sprintf(", database %q ready", n.Database)
			if n.Username != "" && n.Username != "root" && n.Username != "postgres" {
				line += fmt.Sprintf(" for user %q", n.Username)
			}
		}
		fmt.Println(line)
	}
	if root != "" {
		if site, ok := sites.For(home.LoadConfig(), root); ok {
			if _, err := services.Web.Start(443); err != nil {
				fmt.Fprintf(os.Stderr, "✗ %s: %v\n", services.Web.Title, err)
				failed = true
			} else {
				line := "✓ " + site.URL()
				if !services.Trusted() {
					line += " (run `l4win trust` once so browsers accept its certificate)"
				}
				fmt.Println(line)
			}
		}
	}
	if phpRedis {
		// Laravel's default Redis client is the phpredis extension.
		if v, err := php.Resolve(root); err != nil {
			fmt.Fprintf(os.Stderr, "✗ redis PHP extension: %v\n", err)
			failed = true
		} else if err := php.InstallRedis(v); err != nil {
			fmt.Fprintf(os.Stderr, "✗ redis PHP extension: %v\n", err)
			failed = true
		} else {
			fmt.Printf("✓ PHP %s has the redis extension (REDIS_CLIENT=phpredis)\n", v)
		}
	}
	for _, note := range notes {
		fmt.Println("! " + note)
	}
	if len(needs) > 0 && !failed {
		fmt.Println("They keep running in the background; stop them with `l4win down`.")
	}
	if failed {
		return 1
	}
	return 0
}

// cmdDown stops the named services, or all of them.
func cmdDown(args []string) int {
	list := services.All()
	if len(args) > 0 {
		list = nil
		for _, name := range args {
			s, ok := services.Get(name)
			if !ok {
				return fail(fmt.Errorf("unknown service %q (available: %s)", name, services.Names()))
			}
			list = append(list, s)
		}
	}
	stopped := 0
	for _, s := range list {
		was, err := s.Stop()
		if err != nil {
			return fail(err)
		}
		if was {
			fmt.Printf("Stopped %s.\n", s.Title)
			stopped++
		}
	}
	if stopped == 0 {
		fmt.Println("Nothing was running.")
	}
	return 0
}

// cmdDB handles `l4win db create NAME [--user=U] [--password=P]`.
func cmdDB(args []string) int {
	if len(args) < 2 || args[0] != "create" {
		return fail(fmt.Errorf("usage: l4win db create NAME [--user=USER] [--password=PASSWORD]"))
	}
	name, user, password, only := args[1], "", "", ""
	for _, a := range args[2:] {
		switch {
		case strings.HasPrefix(a, "--user="):
			user = strings.TrimPrefix(a, "--user=")
		case strings.HasPrefix(a, "--password="):
			password = strings.TrimPrefix(a, "--password=")
		case strings.HasPrefix(a, "--service="):
			only = strings.ToLower(strings.TrimPrefix(a, "--service="))
		default:
			return fail(fmt.Errorf("db create: unknown option %s", a))
		}
	}
	if only != "" {
		s, ok := services.Get(only)
		if !ok || !s.HasDatabases() {
			return fail(fmt.Errorf("--service must be mysql, mariadb or postgres"))
		}
		only = s.Name
	}
	var running []*services.Service
	for _, s := range services.All() {
		if _, ok := s.Running(); ok && s.HasDatabases() && (only == "" || only == s.Name) {
			running = append(running, s)
		}
	}
	switch {
	case len(running) == 0 && only != "":
		return fail(fmt.Errorf("%s isn't running (l4win up %s)", only, only))
	case len(running) == 0:
		return fail(fmt.Errorf("start a database first: l4win up mysql, mariadb or postgres"))
	case len(running) > 1:
		var names []string
		for _, s := range running {
			names = append(names, s.Name)
		}
		return fail(fmt.Errorf("%s are running; pick one with --service=%s", strings.Join(names, " and "), names[0]))
	}
	if err := running[0].EnsureDatabase(name, user, password); err != nil {
		return fail(err)
	}
	fmt.Printf("Database %q is ready on %s", name, running[0].Title)
	if user != "" && user != "root" && user != "postgres" {
		fmt.Printf(" for user %q", user)
	}
	fmt.Println(".")
	return 0
}

func cmdStatus([]string) int {
	fmt.Printf("%-9s %-13s %-10s %-7s %s\n", "SERVICE", "STATE", "VERSION", "PORT", "DATA")
	for _, s := range services.All() {
		ver := s.Installed()
		state, port := "stopped", "-"
		if st, ok := s.Running(); ok {
			state, port, ver = "running", strconv.Itoa(st.Port), st.Version
		}
		data := "-"
		if ver == "" {
			ver, state = "-", "not installed"
		} else {
			data = s.DataDir(ver)
		}
		fmt.Printf("%-9s %-13s %-10s %-7s %s\n", s.Name, state, ver, port, data)
	}
	fmt.Printf("\nLogs: %s\n", home.Path("logs"))
	return 0
}
