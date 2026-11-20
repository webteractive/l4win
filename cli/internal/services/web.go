package services

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/webteractive/l4win/cli/internal/fetch"
	"github.com/webteractive/l4win/cli/internal/home"
	"github.com/webteractive/l4win/cli/internal/php"
	"github.com/webteractive/l4win/cli/internal/proc"
	"github.com/webteractive/l4win/cli/internal/sites"
	"github.com/webteractive/l4win/cli/internal/version"
)

// Web serves the sites (`l4win park`, `l4win link`) at https://NAME.localhost.
// Caddy (official build) terminates HTTPS with its own local certificate
// authority and passes PHP requests over FastCGI to php-cgi.exe workers,
// which ship with every PHP for Windows: a few per PHP version, so each
// site runs on the PHP its project asks for. A hidden l4win process
// supervises them all: it restarts workers that exit (php-cgi stops after
// PHP_FCGI_MAX_REQUESTS requests) and takes everything down when stopped.
var Web = &Service{
	Name:        "web",
	Title:       "Web (Caddy + PHP)",
	Product:     "Caddy",
	DefaultPort: 443,
	Exe:         "caddy.exe",
	latest:      latestCaddy,
	series:      func(string) string { return "" },
	start:       startWeb,
}

// WorkersPerPHP is how many php-cgi processes each PHP version gets. Each
// handles one request at a time.
const WorkersPerPHP = 4

const firstWorkerPort = 9100

// Upstream is a site with the php-cgi ports that serve it.
type Upstream struct {
	Site  sites.Site
	PHP   version.Version
	Ports []int
}

// Caddyfile builds Caddy's configuration. Without redirects (port 80 is
// taken), http:// addresses simply aren't served.
func Caddyfile(storage string, ups []Upstream, redirects bool) string {
	q := func(p string) string { return `"` + filepath.ToSlash(p) + `"` }
	var b strings.Builder
	b.WriteString("# Written by l4win on every start; changes here are overwritten.\n{\n")
	b.WriteString("\tadmin off\n")
	fmt.Fprintf(&b, "\tstorage file_system %s\n", q(storage))
	b.WriteString("\tlocal_certs\n")
	b.WriteString("\tskip_install_trust\n") // `l4win trust` does it, for the user only
	if !redirects {
		b.WriteString("\tauto_https disable_redirects\n")
	}
	b.WriteString("}\n")
	for _, u := range ups {
		var addrs []string
		for _, p := range u.Ports {
			addrs = append(addrs, "127.0.0.1:"+strconv.Itoa(p))
		}
		fmt.Fprintf(&b, "\n# %s (PHP %s)\n%s.localhost {\n", u.Site.Path, u.PHP, u.Site.Name)
		fmt.Fprintf(&b, "\troot * %s\n", q(u.Site.Root))
		b.WriteString("\tencode zstd gzip\n")
		fmt.Fprintf(&b, "\tphp_fastcgi %s {\n", strings.Join(addrs, " "))
		b.WriteString("\t\tlb_policy least_conn\n") // php-cgi handles one request at a time
		b.WriteString("\t\tlb_try_duration 10s\n")  // ride out a worker restarting
		b.WriteString("\t\tlb_try_interval 100ms\n")
		b.WriteString("\t}\n\tfile_server\n}\n")
	}
	return b.String()
}

// WebProc is one process the web supervisor keeps running.
type WebProc struct {
	Name string   `json:"name"`
	Exe  string   `json:"exe"`
	Args []string `json:"args"`
	Env  []string `json:"env"`
	Dir  string   `json:"dir"`
}

// WebPlan is what `l4win up web` hands to the supervisor.
type WebPlan struct {
	Log   string    `json:"log"`
	Procs []WebProc `json:"procs"`
}

func planFile() string { return home.Path("run", "web-plan.json") }

func startWeb(s *Service, port int) (State, error) {
	if port != 443 && port != 0 {
		return State{}, fmt.Errorf("the web service always uses port 443 (browsers expect https://NAME.localhost there)")
	}
	port = 443
	list := sites.All(home.LoadConfig())
	if len(list) == 0 {
		return State{}, fmt.Errorf("there are no sites yet. In a project run `l4win link`, or in a folder of projects run `l4win park`")
	}
	if holder := portHolder(443); holder != "" {
		return State{}, fmt.Errorf("port 443 is already in use by %s. Stop that program (IIS, Skype, another web server) and try again", holder)
	}
	redirects := portHolder(80) == ""

	ver := s.Installed()
	if ver == "" {
		var err error
		if ver, err = s.Install(); err != nil {
			return State{}, err
		}
	}

	// One worker pool per PHP version the sites use.
	pools := map[version.Version][]int{}
	nextPort := firstWorkerPort
	var ups []Upstream
	for _, site := range list {
		v, err := php.Resolve(site.Path)
		if err != nil {
			return State{}, fmt.Errorf("site %s: %w", site.Name, err)
		}
		if !home.Exists(filepath.Join(php.Dir(v), "php-cgi.exe")) {
			return State{}, fmt.Errorf("PHP %s has no php-cgi.exe", v)
		}
		if _, ok := pools[v]; !ok {
			for len(pools[v]) < WorkersPerPHP {
				if nextPort > 65000 {
					return State{}, fmt.Errorf("no free ports for PHP workers")
				}
				if portHolder(nextPort) == "" {
					pools[v] = append(pools[v], nextPort)
				}
				nextPort++
			}
		}
		ups = append(ups, Upstream{Site: site, PHP: v, Ports: pools[v]})
	}

	for _, dir := range []string{home.Path("etc"), home.Path("run"), home.Path("logs"), s.DataDir(ver)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return State{}, err
		}
	}
	caddyfile := home.Path("etc", "Caddyfile")
	if err := os.WriteFile(caddyfile, []byte(Caddyfile(s.DataDir(ver), ups, redirects)), 0o644); err != nil {
		return State{}, err
	}

	plan := WebPlan{Log: s.LogFile()}
	plan.Procs = append(plan.Procs, WebProc{
		Name: "caddy",
		Exe:  filepath.Join(s.Dir(ver), "caddy.exe"),
		Args: []string{"run", "--config", caddyfile, "--adapter", "caddyfile"},
		Dir:  s.DataDir(ver),
	})
	for v, ports := range pools {
		for _, p := range ports {
			plan.Procs = append(plan.Procs, WebProc{
				Name: fmt.Sprintf("php-cgi %s :%d", v, p),
				Exe:  filepath.Join(php.Dir(v), "php-cgi.exe"),
				Args: []string{"-b", "127.0.0.1:" + strconv.Itoa(p)},
				Env:  append(php.Env(v), "PHP_FCGI_MAX_REQUESTS=500"),
				Dir:  php.Dir(v),
			})
		}
	}
	data, _ := json.MarshalIndent(plan, "", "  ")
	if err := os.WriteFile(planFile(), data, 0o644); err != nil {
		return State{}, err
	}

	self, err := os.Executable()
	if err != nil {
		return State{}, err
	}
	pid, err := startDetached(self, []string{"__web-daemon", planFile()}, "", nil)
	if err != nil {
		return State{}, err
	}

	firstWorker := ups[0].Ports[0]
	deadline := time.Now().Add(30 * time.Second)
	for !listening(443) || !listening(firstWorker) {
		if !processAlive(pid, self) {
			return State{}, fmt.Errorf("the web server stopped while starting. Last lines of %s:\n%s", s.LogFile(), tail(s.LogFile(), 15))
		}
		if time.Now().After(deadline) {
			killProcess(pid)
			return State{}, fmt.Errorf("the web server didn't start within 30s. See %s:\n%s", s.LogFile(), tail(s.LogFile(), 15))
		}
		time.Sleep(250 * time.Millisecond)
	}

	st := State{PID: pid, Port: 443, Version: ver, Exe: self, Started: time.Now().Format(time.RFC3339)}
	return st, s.saveState(st)
}

func (s *Service) saveState(st State) error {
	data, _ := json.MarshalIndent(st, "", "  ")
	return os.WriteFile(s.stateFile(), data, 0o644)
}

// RunWebDaemon is the hidden supervisor (`l4win __web-daemon PLAN`). It
// never returns while things run; stopping the web service kills it, and
// its Job Object takes Caddy and the workers with it.
func RunWebDaemon(planPath string) error {
	if err := proc.TieChildrenToMe(); err != nil {
		return fmt.Errorf("can't tie the web server's processes to l4win: %w", err)
	}
	data, err := os.ReadFile(planPath)
	if err != nil {
		return err
	}
	var plan WebPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return err
	}
	logFile, err := os.OpenFile(plan.Log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	log := &lockedWriter{w: logFile}
	fmt.Fprintf(log, "l4win: web server starting (%d processes)\n", len(plan.Procs))

	for _, p := range plan.Procs {
		go supervise(p, log)
	}
	select {}
}

// supervise keeps one process running, backing off if it keeps failing.
func supervise(p WebProc, log io.Writer) {
	backoff := 500 * time.Millisecond
	for {
		cmd := exec.Command(p.Exe, p.Args...)
		cmd.Dir = p.Dir
		cmd.Env = append(os.Environ(), p.Env...)
		cmd.Stdout, cmd.Stderr = log, log
		started := time.Now()
		err := cmd.Run()
		if time.Since(started) > 10*time.Second {
			backoff = 500 * time.Millisecond // it ran fine for a while
		} else {
			fmt.Fprintf(log, "l4win: %s exited quickly (%v); retrying in %s\n", p.Name, err, backoff)
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(b)
}

// CARoot is Caddy's local root certificate, created on Caddy's first start.
func CARoot() string {
	return filepath.Join(Web.DataDir(""), "pki", "authorities", "local", "root.crt")
}

func readCARoot() (*x509.Certificate, error) {
	data, err := os.ReadFile(CARoot())
	if err != nil {
		return nil, fmt.Errorf("no local certificate authority yet; start the web server once with `l4win up web`")
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("%s isn't a PEM certificate", CARoot())
	}
	return x509.ParseCertificate(block.Bytes)
}

// Trusted reports whether Windows trusts the local certificate authority.
func Trusted() bool {
	cert, err := readCARoot()
	if err != nil {
		return false
	}
	// On Windows, Go verifies against the system and user certificate stores.
	_, err = cert.Verify(x509.VerifyOptions{})
	return err == nil
}

// Trust adds the local certificate authority to the current user's
// trusted roots. Windows asks the user to confirm; no admin rights needed.
func Trust() error {
	if _, err := readCARoot(); err != nil {
		return err
	}
	out, err := exec.Command("certutil", "-user", "-addstore", "Root", CARoot()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("certutil: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Untrust removes it again.
func Untrust() error {
	cert, err := readCARoot()
	if err != nil {
		return err
	}
	serial := fmt.Sprintf("%x", cert.SerialNumber)
	out, err := exec.Command("certutil", "-user", "-delstore", "Root", serial).CombinedOutput()
	if err != nil {
		return fmt.Errorf("certutil: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func latestCaddy() (string, string, string, error) {
	var release struct {
		Tag string `json:"tag_name"`
	}
	if err := fetch.JSON("https://api.github.com/repos/caddyserver/caddy/releases/latest", &release); err != nil {
		return "", "", "", err
	}
	ver := strings.TrimPrefix(release.Tag, "v")
	base := "https://github.com/caddyserver/caddy/releases/download/" + release.Tag + "/"
	name := "caddy_" + ver + "_windows_amd64.zip"
	sums, err := fetch.Text(base + "caddy_" + ver + "_checksums.txt") // SHA-512
	if err != nil {
		return "", "", "", fmt.Errorf("reading Caddy's checksums: %w", err)
	}
	for _, line := range strings.Split(sums, "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] == name {
			return ver, base + name, f[0], nil
		}
	}
	return "", "", "", fmt.Errorf("Caddy %s has no checksum for %s", ver, name)
}
