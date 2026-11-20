// Package project reads a Laravel project's .env and works out which
// services it needs.
package project

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/webteractive/l4win/cli/internal/home"
)

// Root walks up from dir to the folder holding artisan, or "".
func Root(dir string) string {
	dir, _ = filepath.Abs(dir)
	for {
		if home.Exists(filepath.Join(dir, "artisan")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

var interpolation = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

// ReadEnv parses a .env file the way Laravel (phpdotenv) reads common
// files: comments, quotes, "export ", and ${VAR} references.
func ReadEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	env := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch {
		case len(value) >= 2 && value[0] == '\'' && strings.LastIndex(value, "'") > 0:
			value = value[1:strings.LastIndex(value, "'")] // literal
		case len(value) >= 2 && value[0] == '"' && strings.LastIndex(value, `"`) > 0:
			value = value[1:strings.LastIndex(value, `"`)]
			value = strings.NewReplacer(`\"`, `"`, `\n`, "\n", `\\`, `\`).Replace(value)
			value = expand(value, env)
		default:
			if i := strings.Index(value, " #"); i >= 0 {
				value = strings.TrimSpace(value[:i])
			}
			value = expand(value, env)
		}
		env[key] = value
	}
	return env, sc.Err()
}

func expand(value string, env map[string]string) string {
	return interpolation.ReplaceAllStringFunc(value, func(m string) string {
		name := m[2 : len(m)-1]
		if v, ok := env[name]; ok {
			return v
		}
		return os.Getenv(name)
	})
}

// Need is one service the project needs.
type Need struct {
	Service  string // "mysql", "mariadb", "mailpit"
	Port     int
	Database string // for databases
	Username string
	Password string
}

// Plan is what `l4win up` will do for a project.
type Plan struct {
	Needs    []Need
	Notes    []string // things l4win can't do yet, or settings to change
	PHPRedis bool     // the app talks to Redis through the phpredis extension
}

func local(host string) bool {
	switch strings.ToLower(host) {
	case "", "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

func port(value string, fallback int) int {
	if p, err := strconv.Atoi(value); err == nil && p > 0 {
		return p
	}
	return fallback
}

// PlanFor decides which services a .env needs.
func PlanFor(env map[string]string) Plan {
	var p Plan

	switch conn := strings.ToLower(env["DB_CONNECTION"]); conn {
	case "mysql", "mariadb", "pgsql":
		host := env["DB_HOST"]
		if !local(host) {
			p.Notes = append(p.Notes, "DB_HOST="+host+" isn't this machine (Sail's container name?), so l4win won't start a database. Set DB_HOST=127.0.0.1 to use l4win's.")
			break
		}
		// The same defaults as config/database.php.
		db := env["DB_DATABASE"]
		if _, set := env["DB_DATABASE"]; !set {
			db = "laravel"
		}
		user := env["DB_USERNAME"]
		if _, set := env["DB_USERNAME"]; !set {
			user = "root"
		}
		service, defaultPort := conn, 3306
		if conn == "pgsql" {
			service, defaultPort = "postgres", 5432
			if env["DB_PORT"] == "3306" {
				p.Notes = append(p.Notes, "DB_PORT=3306 is MySQL's port; PostgreSQL usually uses 5432. l4win starts PostgreSQL on the port .env says.")
			}
		}
		p.Needs = append(p.Needs, Need{Service: service, Port: port(env["DB_PORT"], defaultPort),
			Database: db, Username: user, Password: env["DB_PASSWORD"]})
	case "sqlsrv":
		p.Notes = append(p.Notes, "SQL Server (DB_CONNECTION=sqlsrv) isn't managed by l4win.")
	}

	if strings.EqualFold(env["MAIL_MAILER"], "smtp") {
		host := env["MAIL_HOST"]
		switch {
		case local(host):
			p.Needs = append(p.Needs, Need{Service: "mailpit", Port: port(env["MAIL_PORT"], 1025)})
		case strings.EqualFold(host, "mailpit"):
			p.Notes = append(p.Notes, "MAIL_HOST=mailpit is Sail's container name. Set MAIL_HOST=127.0.0.1 to use l4win's Mailpit.")
		}
	}

	usesRedis := false
	for _, key := range []string{"CACHE_STORE", "CACHE_DRIVER", "QUEUE_CONNECTION", "SESSION_DRIVER", "BROADCAST_CONNECTION"} {
		usesRedis = usesRedis || strings.EqualFold(env[key], "redis")
	}
	if usesRedis {
		host := env["REDIS_HOST"]
		if local(host) {
			p.Needs = append(p.Needs, Need{Service: "redis", Port: port(env["REDIS_PORT"], 6379)})
			p.PHPRedis = !strings.EqualFold(env["REDIS_CLIENT"], "predis")
			if pw := env["REDIS_PASSWORD"]; pw != "" && !strings.EqualFold(pw, "null") {
				p.Notes = append(p.Notes, "REDIS_PASSWORD is set, but l4win's Redis has no password (it only listens on 127.0.0.1). Set REDIS_PASSWORD=null.")
			}
		} else {
			p.Notes = append(p.Notes, "REDIS_HOST="+host+" isn't this machine (Sail's container name?), so l4win won't start Redis. Set REDIS_HOST=127.0.0.1 to use l4win's.")
		}
	}
	return p
}
