package services

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/webteractive/l4win/cli/internal/fetch"
)

// MailpitUIPort is where Mailpit's inbox is served.
const MailpitUIPort = 8025

// Mailpit catches the app's outgoing mail (MAIL_MAILER=smtp,
// MAIL_HOST=127.0.0.1, MAIL_PORT=1025) and shows it at localhost:8025.
var Mailpit = &Service{
	Name:        "mailpit",
	Title:       "Mailpit",
	DefaultPort: 1025,
	Exe:         "mailpit.exe",
	latest:      latestMailpit,
	series:      func(string) string { return "" }, // one data folder for all versions
	args: func(s *Service, ver string, port int) []string {
		return []string{
			"--smtp", "127.0.0.1:" + strconv.Itoa(port),
			"--listen", "127.0.0.1:" + strconv.Itoa(MailpitUIPort),
			"--database", filepath.Join(s.DataDir(ver), "mailpit.db"),
		}
	},
}

func latestMailpit() (string, string, string, error) {
	var release struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err := fetch.JSON("https://api.github.com/repos/axllent/mailpit/releases/latest", &release); err != nil {
		return "", "", "", err
	}
	for _, a := range release.Assets {
		if a.Name == "mailpit-windows-amd64.zip" {
			sha, ok := strings.CutPrefix(a.Digest, "sha256:")
			if !ok {
				return "", "", "", fmt.Errorf("Mailpit %s has no published SHA-256 for %s", release.Tag, a.Name)
			}
			return strings.TrimPrefix(release.Tag, "v"), a.URL, sha, nil
		}
	}
	return "", "", "", fmt.Errorf("Mailpit %s has no Windows x64 build", release.Tag)
}
