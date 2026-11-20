package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/webteractive/l4win/cli/internal/composer"
	"github.com/webteractive/l4win/cli/internal/home"
	"github.com/webteractive/l4win/cli/internal/node"
	"github.com/webteractive/l4win/cli/internal/php"
	"github.com/webteractive/l4win/cli/internal/services"
	"github.com/webteractive/l4win/cli/internal/setup"
	"github.com/webteractive/l4win/cli/internal/version"
)

func cmdSetup(args []string) int {
	noPath, undo := false, false
	for _, a := range args {
		switch a {
		case "--no-path":
			noPath = true
		case "--undo":
			undo = true
		default:
			return fail(fmt.Errorf("setup: unknown option %s", a))
		}
	}

	if undo {
		for _, s := range services.All() {
			if was, err := s.Stop(); err != nil {
				return fail(err)
			} else if was {
				fmt.Printf("Stopped %s.\n", s.Title)
			}
		}
		changed, err := setup.RemoveFromUserPath(home.Bin())
		if err != nil {
			return fail(err)
		}
		if changed {
			fmt.Printf("Removed %s from your PATH. Open a new terminal for it to apply.\n", home.Bin())
		} else {
			fmt.Println("l4win wasn't on your PATH.")
		}
		if services.Trusted() {
			fmt.Println("Your browsers still trust l4win's local certificates; `l4win trust --undo` removes that.")
		}
		fmt.Printf("To remove everything l4win installed, delete %s\n", home.Dir())
		return 0
	}

	if code := cmdInstall(nil); code != 0 {
		return code
	}
	if err := setup.InstallShims(); err != nil {
		return fail(err)
	}
	fmt.Printf("\nCommands installed in %s\n", home.Bin())

	if noPath {
		fmt.Println("Skipped changing your PATH (--no-path).")
		return 0
	}
	changed, err := setup.AddToUserPath(home.Bin())
	if err != nil {
		return fail(fmt.Errorf("adding %s to your PATH: %w", home.Bin(), err))
	}
	if changed {
		fmt.Println("Added it to the front of your user PATH. Open a new terminal to use php, composer, node, npm and npx.")
	} else {
		fmt.Println("It's already on your PATH.")
	}
	warnConflicts()
	return 0
}

func warnConflicts() {
	conflicts := setup.Conflicts()
	if len(conflicts) == 0 {
		return
	}
	fmt.Println("\nThese will still run instead of l4win's, because they come first on the PATH:")
	for _, c := range conflicts {
		fmt.Printf("  %-9s %s\n", c.Command, c.Path)
	}
	fmt.Println("Windows puts the system PATH before the user PATH. Remove or reorder those entries\n" +
		"(System Properties > Environment Variables), or use `l4win php`, `l4win composer`… meanwhile.")
}

func cmdInstall(args []string) int {
	if len(args) == 0 {
		args = []string{"php", "composer", "node"}
	}
	for _, arg := range args {
		name, ver := splitTool(arg)
		var err error
		switch name {
		case "php":
			_, err = php.Install(ver)
		case "composer":
			err = composer.Install()
		case "node", "nodejs":
			_, err = node.Install(ver)
		case "vcredist":
			if !php.MissingVCRuntime() && ver != "force" {
				fmt.Fprintln(os.Stderr, "The Visual C++ runtime is already installed.")
				continue
			}
			err = php.InstallVCRedist()
		case "ca", "cacert":
			err = php.UpdateCABundle()
		case "phpredis", "php-redis":
			// For PHP versions installed before l4win added it automatically.
			for _, v := range php.Installed() {
				if err = php.InstallRedis(v); err != nil {
					break
				}
				fmt.Fprintf(os.Stderr, "PHP %s has the redis extension.\n", v)
			}
		default:
			if s, ok := services.Get(name); ok {
				_, err = s.Install()
				break
			}
			err = fmt.Errorf("don't know how to install %q (try php, php@8.3, composer, node, node@22, %s, vcredist or ca)", arg, services.Names())
		}
		if err != nil {
			return fail(err)
		}
	}
	return 0
}

func cmdUninstall(args []string) int {
	if len(args) == 0 {
		return fail(fmt.Errorf("uninstall what? For example: l4win uninstall php@8.2"))
	}
	for _, arg := range args {
		name, ver := splitTool(arg)
		if s, ok := services.Get(name); ok {
			if err := s.Uninstall(); err != nil {
				return fail(err)
			}
			fmt.Printf("Removed %s. Its data is kept in %s\n", s.Title, home.Path("data"))
			continue
		}
		if ver == "" {
			return fail(fmt.Errorf("say which version, for example %s@8.2", name))
		}
		var removed []version.Version
		var err error
		switch name {
		case "php":
			removed, err = php.Uninstall(ver)
		case "node", "nodejs":
			removed, err = node.Uninstall(ver)
		default:
			err = fmt.Errorf("can't uninstall %q", arg)
		}
		for _, v := range removed {
			fmt.Printf("Removed %s %s\n", name, v)
		}
		if err != nil {
			return fail(err)
		}
		if len(removed) == 0 {
			fmt.Printf("No installed %s matches %s\n", name, ver)
		}
	}
	return 0
}

func cmdUse(args []string) int {
	if len(args) == 0 {
		return fail(fmt.Errorf("use what? For example: l4win use php@8.4"))
	}
	cfg := home.LoadConfig()
	for _, arg := range args {
		name, ver := splitTool(arg)
		if ver == "" {
			return fail(fmt.Errorf("say which version, for example %s@8.4", name))
		}
		if _, err := version.ParseConstraint(strings.TrimPrefix(ver, "v")); err != nil {
			return fail(err)
		}
		switch name {
		case "php":
			cfg.PHP = ver
			if !anyMatches(php.Installed(), ver) {
				if _, err := php.Install(ver); err != nil {
					return fail(err)
				}
			}
		case "node", "nodejs":
			cfg.Node = ver
			if !anyMatches(node.Installed(), ver) {
				if _, err := node.Install(ver); err != nil {
					return fail(err)
				}
			}
		default:
			return fail(fmt.Errorf("can't set a default for %q", arg))
		}
		fmt.Printf("Default %s is now %s (projects that ask for another version still get theirs).\n", name, ver)
	}
	if err := home.SaveConfig(cfg); err != nil {
		return fail(err)
	}
	return 0
}

func anyMatches(installed []version.Version, want string) bool {
	c, err := version.ParseConstraint(strings.TrimPrefix(want, "v"))
	if err != nil {
		return false
	}
	for _, v := range installed {
		if c.Check(v) {
			return true
		}
	}
	return false
}

func cmdList([]string) int {
	cfg := home.LoadConfig()

	fmt.Println("PHP")
	phps := php.Installed()
	if len(phps) == 0 {
		fmt.Println("  (none: run `l4win install php`)")
	}
	def, _ := php.Default(phps)
	for _, v := range phps {
		mark := ""
		if v == def {
			mark = "  (default)"
		}
		fmt.Printf("  %s%s\n", v, mark)
	}

	fmt.Println("Node.js")
	nodes := node.Installed()
	if len(nodes) == 0 {
		fmt.Println("  (none: run `l4win install node`)")
	}
	for i, v := range nodes {
		mark := ""
		if (cfg.Node != "" && anyMatches([]version.Version{v}, cfg.Node)) || (cfg.Node == "" && i == 0) {
			mark = "  (default)"
		}
		fmt.Printf("  %s%s\n", v, mark)
	}

	fmt.Println("Composer")
	if composer.Installed() {
		fmt.Println("  installed")
	} else {
		fmt.Println("  (none: run `l4win install composer`)")
	}
	return 0
}

func cmdWhich(args []string) int {
	which := []string{"php", "composer", "node"}
	if len(args) > 0 {
		which = args
	}
	dir := cwd()
	for _, tool := range which {
		switch tool {
		case "php", "composer":
			req := php.Find(dir)
			v, err := php.Resolve(dir)
			if err != nil {
				return fail(err)
			}
			why := "default"
			if req.Source != "" {
				why = fmt.Sprintf("%s asks for %s", req.Source, req.Constraint)
			}
			if tool == "composer" {
				fmt.Printf("composer  %s  on PHP %s\n", composer.Phar(), v)
			} else {
				fmt.Printf("php       %s  %s  (%s)\n", v, php.Exe(v), why)
			}
		case "node", "npm", "npx":
			v, err := node.Resolve(dir)
			if err != nil {
				return fail(err)
			}
			fmt.Printf("%-9s %s  %s\n", tool, v, node.Exe(v))
		default:
			return fail(fmt.Errorf("unknown tool %q", tool))
		}
	}
	return 0
}
