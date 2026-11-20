package services

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/webteractive/l4win/cli/internal/dotnet"
	"github.com/webteractive/l4win/cli/internal/fetch"
)

// Redis has no official Windows build. l4win runs Microsoft's Garnet, an
// open-source (MIT) server that speaks the Redis protocol, which is what
// Laravel's cache, session, lock and queue drivers use. It's a .NET
// program, so l4win also downloads the .NET runtime for it (privately).
var Redis = &Service{
	Name:        "redis",
	Title:       "Redis (Garnet)",
	DefaultPort: 6379,
	Exe:         "GarnetServer.exe",
	latest:      latestGarnet,
	series:      func(string) string { return "" }, // one data folder
	env: func(s *Service, ver string) ([]string, error) {
		dir, err := dotnet.Ensure(garnetDotnetChannel(s.Dir(ver)))
		if err != nil {
			return nil, fmt.Errorf("installing the .NET runtime Garnet needs: %w", err)
		}
		return dotnet.Env(dir), nil
	},
	args: func(s *Service, ver string, port int) []string {
		return []string{
			"--port", strconv.Itoa(port),
			"--bind", "127.0.0.1",
			"--lua",                           // Laravel's queue and lock drivers use Lua scripts
			"--checkpointdir", s.DataDir(ver), // where SAVE writes, like Redis's dump.rdb
			"--recover",                          // load the last save on start
			"--memory", "512m", "--index", "64m", // the defaults (16 GB) suit servers, not laptops
		}
	},
	shutdown: saveThenStop,
}

// errSaved tells Stop the data is saved and the process can be ended now:
// Garnet has no SHUTDOWN command.
var errSaved = errors.New("saved")

// saveThenStop asks Garnet to write a checkpoint, as Redis saves on
// shutdown, so cached data and queued jobs survive `l4win down`.
func saveThenStop(s *Service, st State) error {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(st.Port), 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	if _, err := conn.Write([]byte("*1\r\n$4\r\nSAVE\r\n")); err != nil {
		return err
	}
	reply, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.HasPrefix(reply, "+") {
		fmt.Fprintf(os.Stderr, "l4win: Garnet didn't save: %s\n", strings.TrimSpace(reply))
	}
	return errSaved
}

// garnetDotnetChannel reads which .NET version the build targets ("10.0")
// from the runtimeconfig file that ships next to the server.
func garnetDotnetChannel(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "Service", "Garnet.worker.runtimeconfig.json"))
	if err == nil {
		var cfg struct {
			RuntimeOptions struct {
				Framework struct {
					Version string `json:"version"`
				} `json:"framework"`
			} `json:"runtimeOptions"`
		}
		if json.Unmarshal(data, &cfg) == nil {
			if parts := strings.SplitN(cfg.RuntimeOptions.Framework.Version, ".", 3); len(parts) >= 2 {
				return parts[0] + "." + parts[1]
			}
		}
	}
	return "10.0"
}

func latestGarnet() (string, string, string, error) {
	var release struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err := fetch.JSON("https://api.github.com/repos/microsoft/garnet/releases/latest", &release); err != nil {
		return "", "", "", err
	}
	for _, a := range release.Assets {
		if a.Name == "win-x64-based-readytorun.zip" {
			sha, ok := strings.CutPrefix(a.Digest, "sha256:")
			if !ok {
				return "", "", "", fmt.Errorf("Garnet %s has no published SHA-256 for %s", release.Tag, a.Name)
			}
			return strings.TrimPrefix(release.Tag, "v"), a.URL, sha, nil
		}
	}
	return "", "", "", fmt.Errorf("Garnet %s has no Windows x64 build", release.Tag)
}
