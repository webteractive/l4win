package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/webteractive/l4win/cli/internal/home"
)

// PostgreSQL's Windows binaries are built by EDB (linked from
// postgresql.org/download/windows). EDB publishes no checksums, so the
// version and the SHA-256 of the official zip are pinned here and updated
// with l4win releases.
const (
	postgresVersion = "18.7"
	postgresBuild   = "1"
	postgresSHA256  = "ead0bbc27fb678fa24dcf86b738981323042abd073c744103bf7f3aed23f9a33"
)

// postgresSuperuser has no password: the server only listens on 127.0.0.1
// and trusts local connections, like root in l4win's MySQL.
const postgresSuperuser = "postgres"

// Postgres is PostgreSQL, for DB_CONNECTION=pgsql.
var Postgres = &Service{
	Name:        "postgres",
	Title:       "PostgreSQL",
	DefaultPort: 5432,
	Exe:         `bin\postgres.exe`,
	latest: func() (string, string, string, error) {
		url := fmt.Sprintf("https://get.enterprisedb.com/postgresql/postgresql-%s-%s-windows-x64-binaries.zip", postgresVersion, postgresBuild)
		return postgresVersion, url, postgresSHA256, nil
	},
	// Data folders are tied to the major version ("18").
	series: func(ver string) string { return strings.SplitN(ver, ".", 2)[0] },
	// The zip also carries pgAdmin, StackBuilder, docs and headers (most of
	// its files); the server and psql don't need them.
	skip: []string{"pgAdmin 4", "StackBuilder", "doc", "include", "symbols"},
	prepare: func(s *Service, ver string) error {
		data := s.DataDir(ver)
		if home.Exists(filepath.Join(data, "PG_VERSION")) {
			return nil
		}
		// Leftovers of a failed first start; initdb needs an empty folder.
		if err := os.RemoveAll(data); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Creating %s's data folder (first start only)...\n", s.Title)
		cmd := exec.Command(filepath.Join(s.Dir(ver), `bin\initdb.exe`),
			"--pgdata="+data, "--username="+postgresSuperuser, "--auth=trust",
			"--encoding=UTF8", "--no-locale")
		out, err := cmd.CombinedOutput()
		if err != nil {
			os.RemoveAll(data)
			return fmt.Errorf("%v\n%s", err, strings.TrimSpace(string(out)))
		}
		return nil
	},
	args: func(s *Service, ver string, port int) []string {
		return []string{
			"-D", s.DataDir(ver),
			"-p", strconv.Itoa(port),
			"-c", "listen_addresses=127.0.0.1",
			"-c", "logging_collector=off", // output goes to l4win's log file
		}
	},
	shutdown: func(s *Service, st State) error {
		// "fast": disconnect clients and stop cleanly.
		return exec.Command(filepath.Join(s.Dir(st.Version), `bin\pg_ctl.exe`),
			"stop", "-D", s.DataDir(st.Version), "-m", "fast", "-w").Run()
	},
	ensureDB: postgresEnsureDB,
}

func postgresQuery(s *Service, st State, sql string) (string, error) {
	cmd := exec.Command(filepath.Join(s.Dir(st.Version), `bin\psql.exe`),
		"--host=127.0.0.1", "--port="+strconv.Itoa(st.Port), "--username="+postgresSuperuser,
		"--dbname=postgres", "--no-psqlrc", "--tuples-only", "--no-align",
		"--set=ON_ERROR_STOP=1", "--command="+sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// postgresEnsureDB creates a login role (unless it's the superuser) and a
// database it owns. CREATE DATABASE can't run in a transaction or DO
// block, so existence is checked first.
func postgresEnsureDB(s *Service, st State, db, user, password string) error {
	ident := func(id string) string { return `"` + strings.ReplaceAll(id, `"`, `""`) + `"` }
	lit := func(v string) string { return "'" + strings.ReplaceAll(v, "'", "''") + "'" }

	owner := postgresSuperuser
	if user != "" && user != postgresSuperuser {
		owner = user
		role := fmt.Sprintf(`DO $l4win$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = %s) THEN
    CREATE ROLE %s LOGIN PASSWORD %s;
  END IF;
END $l4win$;`, lit(user), ident(user), lit(password))
		if _, err := postgresQuery(s, st, role); err != nil {
			return err
		}
	}

	exists, err := postgresQuery(s, st, "SELECT 1 FROM pg_database WHERE datname = "+lit(db))
	if err != nil {
		return err
	}
	if exists == "1" {
		return nil
	}
	_, err = postgresQuery(s, st, fmt.Sprintf("CREATE DATABASE %s OWNER %s ENCODING 'UTF8' TEMPLATE template0", ident(db), ident(owner)))
	return err
}
