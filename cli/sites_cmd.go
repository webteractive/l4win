package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/webteractive/l4win/cli/internal/home"
	"github.com/webteractive/l4win/cli/internal/php"
	"github.com/webteractive/l4win/cli/internal/project"
	"github.com/webteractive/l4win/cli/internal/services"
	"github.com/webteractive/l4win/cli/internal/sites"
)

// saveSites writes the config and, if the web server runs, restarts it so
// the change takes effect.
func saveSites(cfg home.Config) int {
	if err := home.SaveConfig(cfg); err != nil {
		return fail(err)
	}
	if _, running := services.Web.Running(); running {
		if _, err := services.Web.Stop(); err != nil {
			return fail(err)
		}
		if _, err := services.Web.Start(443); err != nil {
			return fail(fmt.Errorf("restarting the web server: %w", err))
		}
		fmt.Println("Restarted the web server.")
	} else {
		fmt.Println("Start the web server with `l4win up web`.")
	}
	return 0
}

func cmdPark(args []string) int {
	dir := cwd()
	if len(args) > 0 {
		dir = args[0]
	}
	cfg := home.LoadConfig()
	added, err := sites.Park(&cfg, dir)
	if err != nil {
		return fail(err)
	}
	if !added {
		fmt.Println("That folder is already parked.")
		return 0
	}
	fmt.Printf("Parked %s. Each Laravel app in it is served at https://FOLDER.localhost:\n", dir)
	n := 0
	for _, s := range sites.All(cfg) {
		if !s.Linked && strings.HasPrefix(strings.ToLower(s.Path), strings.ToLower(cfg.Parked[len(cfg.Parked)-1])) {
			fmt.Printf("  %-32s %s\n", s.URL(), s.Path)
			n++
		}
	}
	if n == 0 {
		fmt.Println("  (no Laravel apps in it yet; new ones are picked up when the web server starts)")
	}
	return saveSites(cfg)
}

func cmdUnpark(args []string) int {
	dir := cwd()
	if len(args) > 0 {
		dir = args[0]
	}
	cfg := home.LoadConfig()
	if !sites.Unpark(&cfg, dir) {
		fmt.Println("That folder isn't parked. `l4win sites` shows what is.")
		return 0
	}
	fmt.Printf("Unparked %s.\n", dir)
	return saveSites(cfg)
}

func cmdLink(args []string) int {
	root := project.Root(cwd())
	if root == "" {
		root = cwd()
	}
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	cfg := home.LoadConfig()
	name, err := sites.Link(&cfg, root, name)
	if err != nil {
		return fail(err)
	}
	site := sites.Site{Name: name}
	fmt.Printf("Linked %s as %s\n", root, site.URL())
	fmt.Printf("Set APP_URL=%s in the project's .env so Laravel builds links with it.\n", site.URL())
	return saveSites(cfg)
}

func cmdUnlink(args []string) int {
	target := cwd()
	if root := project.Root(target); root != "" {
		target = root
	}
	if len(args) > 0 {
		target = args[0]
	}
	cfg := home.LoadConfig()
	removed := sites.Unlink(&cfg, target)
	if len(removed) == 0 {
		fmt.Println("Nothing linked there. `l4win sites` shows the links.")
		return 0
	}
	fmt.Printf("Unlinked %s.\n", strings.Join(removed, ", "))
	return saveSites(cfg)
}

func cmdSites([]string) int {
	cfg := home.LoadConfig()
	list := sites.All(cfg)
	if len(list) == 0 {
		fmt.Println("No sites yet. In a project run `l4win link`, or in a folder of projects `l4win park`.")
		return 0
	}
	fmt.Printf("%-34s %-8s %-7s %s\n", "URL", "PHP", "FROM", "FOLDER")
	for _, s := range list {
		ver := "-"
		if v, ok := php.ResolveInstalled(s.Path); ok {
			ver = v.String()
		}
		from := "parked"
		if s.Linked {
			from = "link"
		}
		fmt.Printf("%-34s %-8s %-7s %s\n", s.URL(), ver, from, s.Path)
	}
	if len(cfg.Parked) > 0 {
		fmt.Printf("\nParked folders: %s\n", strings.Join(cfg.Parked, "; "))
	}
	if _, running := services.Web.Running(); !running {
		fmt.Println("The web server isn't running: `l4win up web`.")
	} else if !services.Trusted() {
		fmt.Println("Browsers will warn about the certificate until you run `l4win trust`.")
	}
	return 0
}

// cmdOpen opens this project's site (or the named one) in the browser.
func cmdOpen(args []string) int {
	cfg := home.LoadConfig()
	var site sites.Site
	found := false
	if len(args) > 0 {
		for _, s := range sites.All(cfg) {
			if s.Name == sites.Name(args[0]) {
				site, found = s, true
			}
		}
	} else if root := project.Root(cwd()); root != "" {
		site, found = sites.For(cfg, root)
	}
	if !found {
		return fail(fmt.Errorf("no site here. Link this project with `l4win link`, or name one: l4win open NAME"))
	}
	if _, running := services.Web.Running(); !running {
		if _, err := services.Web.Start(443); err != nil {
			return fail(err)
		}
	}
	fmt.Println(site.URL())
	return runQuiet("rundll32", "url.dll,FileProtocolHandler", site.URL())
}

func cmdTrust(args []string) int {
	if len(args) > 0 && args[0] == "--undo" {
		if err := services.Untrust(); err != nil {
			return fail(err)
		}
		fmt.Println("Removed l4win's local certificate authority from your trusted certificates.")
		return 0
	}
	if services.Trusted() {
		fmt.Println("Your browsers already trust l4win's local certificates.")
		return 0
	}
	fmt.Println("Adding l4win's local certificate authority (made by Caddy, on this machine only)")
	fmt.Println("to your user's trusted certificates. Windows will ask you to confirm.")
	if err := services.Trust(); err != nil {
		return fail(err)
	}
	if services.Trusted() {
		fmt.Println("Done: https://*.localhost sites are trusted. Restart Firefox if it's open.")
		return 0
	}
	return fail(fmt.Errorf("the certificate wasn't added (was the Windows prompt declined?)"))
}

func runQuiet(name string, args ...string) int {
	if err := exec.Command(name, args...).Run(); err != nil {
		return fail(err)
	}
	return 0
}

// webDaemon is the hidden supervisor started by `l4win up web`.
func webDaemon(args []string) int {
	if len(args) != 1 {
		return fail(fmt.Errorf("usage: l4win __web-daemon PLAN"))
	}
	if err := services.RunWebDaemon(args[0]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
