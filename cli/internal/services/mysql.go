package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/webteractive/l4win/cli/internal/fetch"
	"github.com/webteractive/l4win/cli/internal/home"
)

// MySQL publishes no machine-readable index of its Windows builds, so the
// version is pinned here with the SHA-256 of the official zip, and updated
// with l4win releases. 8.4 is the LTS series (what Sail uses).
const (
	mysqlVersion = "8.4.13"
	mysqlSHA256  = "e9cd98b1d23f122987b1afca26ad942efec5891481103d48ae3cde237fa3ea54"
)

// MySQL is Oracle's MySQL Community Server, for DB_CONNECTION=mysql.
var MySQL = &Service{
	Name:        "mysql",
	Title:       "MySQL",
	DefaultPort: 3306,
	Exe:         `bin\mysqld.exe`,
	ownLog:      true,
	latest: func() (string, string, string, error) {
		series := mysqlVersion[:strings.LastIndex(mysqlVersion, ".")]
		url := fmt.Sprintf("https://cdn.mysql.com/Downloads/MySQL-%s/mysql-%s-winx64.zip", series, mysqlVersion)
		return mysqlVersion, url, mysqlSHA256, nil
	},
	series:   minorSeries,
	prepare:  prepareMySQL,
	args:     mysqlArgs,
	shutdown: mysqlShutdown,
	ensureDB: mysqlEnsureDB,
}

// MariaDB is MariaDB Community Server, for DB_CONNECTION=mariadb. The
// newest long-term-support series is used.
var MariaDB = &Service{
	Name:        "mariadb",
	Title:       "MariaDB",
	DefaultPort: 3306,
	Exe:         `bin\mariadbd.exe`,
	ownLog:      true,
	latest:      latestMariaDB,
	series:      minorSeries,
	prepare:     prepareMySQL,
	args:        mysqlArgs,
	shutdown:    mysqlShutdown,
	ensureDB:    mysqlEnsureDB,
}

func minorSeries(ver string) string {
	parts := strings.SplitN(ver, ".", 3)
	if len(parts) < 2 {
		return ver
	}
	return parts[0] + "." + parts[1]
}

func (s *Service) configFile(ver string) string {
	return home.Path("etc", s.Name+"-"+s.series(ver)+".ini")
}

// prepareMySQL writes the server's config file and creates the system
// tables the first time. root has no password, like Laravel's default .env.
func prepareMySQL(s *Service, ver string) error {
	data := s.DataDir(ver)
	ini := s.configFile(ver)
	if !home.Exists(ini) {
		if err := os.MkdirAll(filepath.Dir(ini), 0o755); err != nil {
			return err
		}
		// basedir, datadir, port and the log file are passed on the command
		// line by l4win, so they follow upgrades and the project's DB_PORT.
		conf := strings.Join([]string{
			"# " + s.Title + " settings for l4win. Edit freely; this file is kept.",
			"[mysqld]",
			"bind-address=127.0.0.1",
			"character-set-server=utf8mb4",
			"collation-server=utf8mb4_unicode_ci",
			"max_allowed_packet=64M",
			"",
		}, "\n")
		if s.Name == "mysql" {
			conf += "mysqlx=OFF\n" // X Protocol isn't used by Laravel
		}
		if err := os.WriteFile(ini, []byte(conf), 0o644); err != nil {
			return err
		}
	}

	// The "mysql" system schema folder exists once initialization worked.
	if home.Exists(filepath.Join(data, "mysql")) {
		return nil
	}
	// Anything else here is left over from a failed first start (there are
	// no databases without the system schema), and initializing needs an
	// empty folder.
	if err := os.RemoveAll(data); err != nil {
		return err
	}
	if err := os.MkdirAll(data, 0o755); err != nil {
		return err
	}

	var cmd *exec.Cmd
	if s.Name == "mysql" {
		cmd = exec.Command(filepath.Join(s.Dir(ver), `bin\mysqld.exe`),
			"--defaults-file="+ini, "--basedir="+s.Dir(ver), "--datadir="+data, "--initialize-insecure", "--console")
	} else {
		cmd = exec.Command(filepath.Join(s.Dir(ver), `bin\mariadb-install-db.exe`), "--datadir="+data)
	}
	fmt.Fprintf(os.Stderr, "Creating %s's data folder (first start only)...\n", s.Title)
	out, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(data)
		return fmt.Errorf("%v\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func mysqlArgs(s *Service, ver string, port int) []string {
	return []string{
		"--defaults-file=" + s.configFile(ver),
		"--basedir=" + s.Dir(ver),
		"--datadir=" + s.DataDir(ver),
		"--port=" + strconv.Itoa(port),
		"--log-error=" + s.LogFile(),
	}
}

func (s *Service) clientExe(ver string) string {
	if s.Name == "mariadb" {
		return filepath.Join(s.Dir(ver), `bin\mariadb.exe`)
	}
	return filepath.Join(s.Dir(ver), `bin\mysql.exe`)
}

func mysqlShutdown(s *Service, st State) error {
	admin := `bin\mysqladmin.exe`
	if s.Name == "mariadb" {
		admin = `bin\mariadb-admin.exe`
	}
	cmd := exec.Command(filepath.Join(s.Dir(st.Version), admin),
		"--protocol=TCP", "--host=127.0.0.1", "--port="+strconv.Itoa(st.Port), "--user=root", "shutdown")
	return cmd.Run()
}

// mysqlSQL runs statements as root on the running server.
func mysqlSQL(s *Service, st State, statements string) error {
	cmd := exec.Command(s.clientExe(st.Version),
		"--protocol=TCP", "--host=127.0.0.1", "--port="+strconv.Itoa(st.Port), "--user=root",
		"--batch", "--execute="+statements)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// mysqlEnsureDB creates a database and, unless it's root, a user with
// full rights on it. Existing databases and users are left as they are.
func mysqlEnsureDB(s *Service, st State, db, user, password string) error {
	q := func(id string) string { return "`" + strings.ReplaceAll(id, "`", "``") + "`" }
	lit := func(v string) string {
		return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v) + "'"
	}
	sql := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;", q(db))
	if user != "" && user != "root" {
		for _, host := range []string{"localhost", "127.0.0.1"} {
			sql += fmt.Sprintf(" CREATE USER IF NOT EXISTS %s@%s IDENTIFIED BY %s; GRANT ALL PRIVILEGES ON %s.* TO %s@%s;",
				lit(user), lit(host), lit(password), q(db), lit(user), lit(host))
		}
		sql += " FLUSH PRIVILEGES;"
	}
	return mysqlSQL(s, st, sql)
}

func latestMariaDB() (string, string, string, error) {
	var majors struct {
		Releases []struct {
			ID      string `json:"release_id"`
			Status  string `json:"release_status"`
			Support string `json:"release_support_type"`
		} `json:"major_releases"`
	}
	if err := fetch.JSON("https://downloads.mariadb.org/rest-api/mariadb/", &majors); err != nil {
		return "", "", "", err
	}
	series := ""
	for _, r := range majors.Releases { // newest first
		if r.Status == "Stable" && r.Support == "Long Term Support" {
			series = r.ID
			break
		}
	}
	if series == "" {
		return "", "", "", fmt.Errorf("no stable LTS series in MariaDB's release list")
	}

	var latest struct {
		Releases map[string]struct {
			Files []struct {
				Name     string `json:"file_name"`
				URL      string `json:"file_download_url"`
				Checksum struct {
					SHA256 string `json:"sha256sum"`
				} `json:"checksum"`
			} `json:"files"`
		} `json:"releases"`
	}
	if err := fetch.JSON("https://downloads.mariadb.org/rest-api/mariadb/"+series+"/latest/", &latest); err != nil {
		return "", "", "", err
	}
	for ver, rel := range latest.Releases {
		for _, f := range rel.Files {
			if f.Name == "mariadb-"+ver+"-winx64.zip" {
				return ver, strings.Replace(f.URL, "http://", "https://", 1), f.Checksum.SHA256, nil
			}
		}
	}
	return "", "", "", fmt.Errorf("no Windows zip for MariaDB %s", series)
}
