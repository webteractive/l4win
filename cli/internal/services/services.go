// Package services runs the background programs a Laravel app talks to
// (MySQL, MariaDB, Mailpit), shared by all projects like Herd's services.
//
// Each service is installed from its official Windows build into
// %LOCALAPPDATA%\l4win\services\<name>\<version>, keeps its data in
// data\<name>-<series>, logs to logs\<name>.log, and runs hidden in the
// background until `l4win down`. Nothing is installed as a Windows service
// and nothing starts with Windows.
package services

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/webteractive/l4win/cli/internal/fetch"
	"github.com/webteractive/l4win/cli/internal/home"
	"github.com/webteractive/l4win/cli/internal/version"
)

// Service is one kind of background server.
type Service struct {
	Name        string // "mysql"
	Title       string // "MySQL"
	DefaultPort int
	Exe         string // main executable, relative to the install folder
	ownLog      bool   // writes its own log file (so console output is discarded)

	// latest returns the version to install, its download URL and SHA-256.
	latest func() (ver, url, sha string, err error)
	// series is the data-compatibility group of a version ("8.4").
	series func(ver string) string
	// prepare initializes the data folder the first time.
	prepare func(s *Service, ver string) error
	// args builds the command line for running on port.
	args func(s *Service, ver string, port int) []string
	// shutdown asks the server to stop (it's killed if this fails).
	shutdown func(s *Service, st State) error
	// env adds environment variables for the server process (optional).
	env func(s *Service, ver string) ([]string, error)
	// skip lists top-level folders of the zip not worth unpacking (optional).
	skip []string
	// ensureDB creates a database and user (databases only).
	ensureDB func(s *Service, st State, db, user, password string) error
	// Product names the program installed, when it isn't the Title.
	Product string
	// start replaces the standard start (the web service runs l4win itself).
	start func(s *Service, port int) (State, error)
}

// All lists the services in display order.
func All() []*Service { return []*Service{MySQL, MariaDB, Postgres, Redis, Mailpit, Web} }

// Get finds a service by name.
func Get(name string) (*Service, bool) {
	name = strings.ToLower(name)
	switch name { // other names people use
	case "garnet":
		name = "redis"
	case "pgsql", "postgresql":
		name = "postgres"
	}
	for _, s := range All() {
		if s.Name == name {
			return s, true
		}
	}
	return nil, false
}

// Names lists service names, for messages.
func Names() string {
	var names []string
	for _, s := range All() {
		names = append(names, s.Name)
	}
	return strings.Join(names, ", ")
}

// Root is where this service's versions are installed.
func (s *Service) Root() string { return home.Path("services", s.Name) }

// Dir is one installed version's folder.
func (s *Service) Dir(ver string) string { return filepath.Join(s.Root(), ver) }

// DataDir is the data folder for the version's series. Versions in one
// series share it; a new series (MySQL 8.4 → 9.x) starts a fresh one.
func (s *Service) DataDir(ver string) string {
	if series := s.series(ver); series != "" {
		return home.Path("data", s.Name+"-"+series)
	}
	return home.Path("data", s.Name)
}

// LogFile is where the service's output goes.
func (s *Service) LogFile() string { return home.Path("logs", s.Name+".log") }

// Installed returns the newest installed version, or "".
func (s *Service) Installed() string {
	entries, _ := os.ReadDir(s.Root())
	var vers []string
	for _, e := range entries {
		if e.IsDir() && home.Exists(filepath.Join(s.Root(), e.Name(), s.Exe)) {
			vers = append(vers, e.Name())
		}
	}
	sort.Slice(vers, func(i, j int) bool {
		a, _ := version.Parse(vers[i])
		b, _ := version.Parse(vers[j])
		return a.Compare(b) > 0
	})
	if len(vers) == 0 {
		return ""
	}
	return vers[0]
}

// Install installs the current version if it isn't already.
func (s *Service) Install() (string, error) {
	ver, url, sha, err := s.latest()
	if err != nil {
		return "", fmt.Errorf("finding the latest %s: %w", s.Title, err)
	}
	if home.Exists(filepath.Join(s.Dir(ver), s.Exe)) {
		fmt.Fprintf(os.Stderr, "%s %s is already installed.\n", s.product(), ver)
		return ver, nil
	}
	fmt.Fprintf(os.Stderr, "Installing %s %s\n", s.product(), ver)
	if err := fetch.Zip(url, sha, s.Dir(ver), filepath.Base(url), s.skip...); err != nil {
		return "", err
	}
	fmt.Fprintf(os.Stderr, "%s %s is ready.\n", s.product(), ver)
	return ver, nil
}

// HasDatabases reports whether EnsureDatabase works for this service.
func (s *Service) HasDatabases() bool { return s.ensureDB != nil }

// EnsureDatabase creates a database and, unless user is the server's
// superuser, a user owning it. Existing ones are left as they are.
func (s *Service) EnsureDatabase(db, user, password string) error {
	if db == "" {
		return nil
	}
	if s.ensureDB == nil {
		return fmt.Errorf("%s has no databases", s.Title)
	}
	st, ok := s.Running()
	if !ok {
		return fmt.Errorf("%s isn't running", s.Title)
	}
	return s.ensureDB(s, st, db, user, password)
}

// Uninstall removes the installed binaries but keeps the data.
func (s *Service) Uninstall() error {
	if st, ok := s.Running(); ok {
		return fmt.Errorf("%s is running (pid %d); run `l4win down %s` first", s.Title, st.PID, s.Name)
	}
	return os.RemoveAll(s.Root())
}

// State is what l4win records about a running service.
type State struct {
	PID     int    `json:"pid"`
	Port    int    `json:"port"`
	Version string `json:"version"`
	Exe     string `json:"exe"`
	Started string `json:"started"`
}

func (s *Service) stateFile() string { return home.Path("run", s.Name+".json") }

// Running returns the service's state if l4win started it and it's alive.
func (s *Service) Running() (State, bool) {
	var st State
	data, err := os.ReadFile(s.stateFile())
	if err != nil || json.Unmarshal(data, &st) != nil {
		return State{}, false
	}
	if !processAlive(st.PID, st.Exe) {
		os.Remove(s.stateFile())
		return State{}, false
	}
	return st, true
}

// Start installs the service if needed and starts it on port, waiting
// until it accepts connections. If it's already running it's left alone.
func (s *Service) Start(port int) (State, error) {
	if port == 0 {
		port = s.DefaultPort
	}
	if st, ok := s.Running(); ok {
		return st, nil
	}
	if s.start != nil {
		return s.start(s, port)
	}
	for _, other := range All() {
		if other == s {
			continue
		}
		if st, ok := other.Running(); ok && st.Port == port {
			return State{}, fmt.Errorf("l4win's %s is using port %d. Stop it with `l4win down %s`, or use another port", other.Title, port, other.Name)
		}
	}
	if holder := portHolder(port); holder != "" {
		return State{}, fmt.Errorf("port %d is already in use by %s.\n  Stop that program, or use another port: set it in the project's .env (%s) or run `l4win up %s:%d`",
			port, holder, s.portSetting(), s.Name, port+1)
	}

	ver := s.Installed()
	if ver == "" {
		var err error
		if ver, err = s.Install(); err != nil {
			return State{}, err
		}
	}
	for _, dir := range []string{s.DataDir(ver), home.Path("logs"), home.Path("run")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return State{}, err
		}
	}
	if s.prepare != nil {
		if err := s.prepare(s, ver); err != nil {
			return State{}, fmt.Errorf("setting up %s's data folder: %w", s.Title, err)
		}
	}

	exe := filepath.Join(s.Dir(ver), s.Exe)
	logFile := s.LogFile()
	if s.ownLog {
		logFile = ""
	}
	var env []string
	if s.env != nil {
		var err error
		if env, err = s.env(s, ver); err != nil {
			return State{}, err
		}
	}
	pid, err := startDetached(exe, s.args(s, ver, port), logFile, env)
	if err != nil {
		return State{}, fmt.Errorf("starting %s: %w", s.Title, err)
	}
	st := State{PID: pid, Port: port, Version: ver, Exe: exe, Started: time.Now().Format(time.RFC3339)}

	deadline := time.Now().Add(60 * time.Second)
	for !listening(port) {
		if !processAlive(pid, exe) {
			return State{}, fmt.Errorf("%s stopped while starting. Last lines of %s:\n%s", s.Title, s.LogFile(), tail(s.LogFile(), 15))
		}
		if time.Now().After(deadline) {
			killProcess(pid)
			return State{}, fmt.Errorf("%s didn't start listening on port %d within 60s. See %s", s.Title, port, s.LogFile())
		}
		time.Sleep(250 * time.Millisecond)
	}

	data, _ := json.MarshalIndent(st, "", "  ")
	if err := os.WriteFile(s.stateFile(), data, 0o644); err != nil {
		return st, err
	}
	return st, nil
}

func (s *Service) portSetting() string {
	if s.Name == "mailpit" {
		return "MAIL_PORT"
	}
	return "DB_PORT"
}

// Stop stops the service if it's running. It reports whether it was.
func (s *Service) Stop() (bool, error) {
	st, ok := s.Running()
	if !ok {
		return false, nil
	}
	// Databases are asked to shut down so they can flush to disk, and wait
	// for it. Anything else is ended right away: services without a shutdown
	// command (Mailpit: SQLite, crash-safe), Garnet once it has saved, or a
	// database whose shutdown request failed.
	grace := 30 * time.Second
	if s.shutdown == nil || s.shutdown(s, st) != nil {
		grace = 0
	}
	deadline := time.Now().Add(grace)
	for processAlive(st.PID, st.Exe) && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
	if processAlive(st.PID, st.Exe) {
		if err := killProcess(st.PID); err != nil {
			return true, fmt.Errorf("stopping %s (pid %d): %w", s.Title, st.PID, err)
		}
	}
	os.Remove(s.stateFile())
	return true, nil
}

func listening(port int) bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// tail returns the last n lines of a file.
func tail(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "  (no log)"
	}
	lines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for i, l := range lines {
		lines[i] = "  " + strings.TrimRight(l, "\r")
	}
	return strings.Join(lines, "\n")
}

func (s *Service) product() string {
	if s.Product != "" {
		return s.Product
	}
	return s.Title
}
