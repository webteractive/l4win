package project

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeEnv(t *testing.T, content string) map[string]string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	env, err := ReadEnv(path)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func TestReadEnv(t *testing.T) {
	env := writeEnv(t, `# comment
APP_NAME="My App"
APP_URL=http://localhost # trailing comment
export DB_DATABASE=demo
MAIL_FROM_NAME="${APP_NAME}"
LITERAL='${APP_NAME} #x'
EMPTY=
`)
	want := map[string]string{
		"APP_NAME":       "My App",
		"APP_URL":        "http://localhost",
		"DB_DATABASE":    "demo",
		"MAIL_FROM_NAME": "My App",
		"LITERAL":        "${APP_NAME} #x",
		"EMPTY":          "",
	}
	if !reflect.DeepEqual(env, want) {
		t.Errorf("ReadEnv = %#v\nwant %#v", env, want)
	}
}

func TestPlanMySQLAndMailpit(t *testing.T) {
	p := PlanFor(map[string]string{
		"DB_CONNECTION": "mysql", "DB_HOST": "127.0.0.1", "DB_PORT": "3307",
		"DB_DATABASE": "shop", "DB_USERNAME": "shop", "DB_PASSWORD": "secret",
		"MAIL_MAILER": "smtp", "MAIL_HOST": "127.0.0.1", "MAIL_PORT": "2525",
		"CACHE_STORE": "redis",
	})
	want := []Need{
		{Service: "mysql", Port: 3307, Database: "shop", Username: "shop", Password: "secret"},
		{Service: "mailpit", Port: 2525},
		{Service: "redis", Port: 6379},
	}
	if !reflect.DeepEqual(p.Needs, want) {
		t.Errorf("Needs = %#v", p.Needs)
	}
	if !p.PHPRedis || len(p.Notes) != 0 {
		t.Errorf("PHPRedis = %v, notes = %v", p.PHPRedis, p.Notes)
	}
}

func TestPlanPostgres(t *testing.T) {
	p := PlanFor(map[string]string{"DB_CONNECTION": "pgsql", "DB_DATABASE": "app", "DB_USERNAME": "app", "DB_PASSWORD": "pw"})
	want := Need{Service: "postgres", Port: 5432, Database: "app", Username: "app", Password: "pw"}
	if len(p.Needs) != 1 || p.Needs[0] != want || len(p.Notes) != 0 {
		t.Errorf("pgsql: %#v", p)
	}

	p = PlanFor(map[string]string{"DB_CONNECTION": "pgsql", "DB_PORT": "3306"})
	if len(p.Needs) != 1 || p.Needs[0].Port != 3306 || len(p.Notes) != 1 {
		t.Errorf("pgsql on MySQL's port: %#v", p)
	}
}

func TestPlanRedis(t *testing.T) {
	p := PlanFor(map[string]string{
		"QUEUE_CONNECTION": "redis", "REDIS_HOST": "127.0.0.1", "REDIS_PORT": "6380",
		"REDIS_CLIENT": "predis", "REDIS_PASSWORD": "null",
	})
	if len(p.Needs) != 1 || p.Needs[0] != (Need{Service: "redis", Port: 6380}) || p.PHPRedis || len(p.Notes) != 0 {
		t.Errorf("predis: %#v", p)
	}

	p = PlanFor(map[string]string{"SESSION_DRIVER": "redis", "REDIS_PASSWORD": "hunter2"})
	if len(p.Needs) != 1 || len(p.Notes) != 1 {
		t.Errorf("password note: %#v", p)
	}

	p = PlanFor(map[string]string{"CACHE_STORE": "redis", "REDIS_HOST": "redis"})
	if len(p.Needs) != 0 || len(p.Notes) != 1 {
		t.Errorf("Sail host: %#v", p)
	}

	// REDIS_HOST alone (Laravel's default .env has it) doesn't start Redis.
	p = PlanFor(map[string]string{"REDIS_HOST": "127.0.0.1", "CACHE_STORE": "database"})
	if len(p.Needs) != 0 {
		t.Errorf("unused Redis: %#v", p)
	}
}

func TestPlanDefaultsAndSail(t *testing.T) {
	p := PlanFor(map[string]string{"DB_CONNECTION": "mariadb"})
	if len(p.Needs) != 1 || p.Needs[0] != (Need{Service: "mariadb", Port: 3306, Database: "laravel", Username: "root"}) {
		t.Errorf("defaults: %#v", p.Needs)
	}

	p = PlanFor(map[string]string{"DB_CONNECTION": "mysql", "DB_HOST": "mysql", "MAIL_MAILER": "smtp", "MAIL_HOST": "mailpit"})
	if len(p.Needs) != 0 || len(p.Notes) != 2 {
		t.Errorf("Sail hosts: needs %v, notes %v", p.Needs, p.Notes)
	}

	p = PlanFor(map[string]string{"DB_CONNECTION": "sqlite", "MAIL_MAILER": "log"})
	if len(p.Needs) != 0 || len(p.Notes) != 0 {
		t.Errorf("sqlite: %#v", p)
	}
}
